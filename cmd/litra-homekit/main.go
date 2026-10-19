package main

import (
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"

	"github.com/brutella/hap"
	"github.com/brutella/hap/accessory"
	"github.com/brutella/hap/characteristic"
	"github.com/jslay88/go-litra-beam"
)

func main() {
	pinFlag := flag.String("pin", "", "HomeKit setup PIN. Eight digits, no dashes. Stored on first use")
	data := flag.String("data", "", "Directory for HomeKit pairing data. Defaults to the user data directory")
	port := flag.String("port", "", "TCP port to listen on. Empty uses a port chosen by the OS")
	reset := flag.Bool("reset", false, "Delete pairing data and exit")
	var installService bool
	if runtime.GOOS == "linux" {
		flag.BoolVar(&installService, "install-service", false, "Install and enable the systemd user service, then exit")
	}
	flag.Parse()

	if installService {
		if err := installServiceConflict(*pinFlag, *port, *data, *reset); err != nil {
			log.Fatal(err)
		}
		exe, err := executablePath()
		if err != nil {
			log.Fatalf("Error finding executable path: %v", err)
		}
		dir, err := userServiceDir()
		if err != nil {
			log.Fatalf("Error finding systemd user directory: %v", err)
		}
		path, err := installUserService(dir, exe, runCommand)
		if err != nil {
			if path != "" {
				log.Fatalf("Wrote %s, but systemctl failed: %v", path, err)
			}
			log.Fatalf("Error installing user service: %v", err)
		}
		log.Printf("Wrote %s\n", path)
		log.Println("Enabled and started litra-homekit.")
		log.Printf("To keep it running after logout: sudo loginctl enable-linger %s\n", currentUser())
		os.Exit(0)
	}

	dataDir := *data
	if dataDir == "" {
		var err error
		dataDir, err = defaultDataDir()
		if err != nil {
			log.Fatalf("Error finding data directory: %v", err)
		}
	}
	if *reset {
		if *pinFlag != "" || *port != "" {
			log.Fatalln("-reset cannot be combined with -pin or -port")
		}
		if err := os.RemoveAll(dataDir); err != nil {
			log.Fatalf("Error deleting pairing data: %v", err)
		}
		log.Printf("Deleted pairing data at %s\n", dataDir)
		log.Println("Remove the bridge in the Home app, then start litra-homekit again to pair.")
		os.Exit(0)
	}

	stored, err := loadPin(dataDir)
	if err != nil {
		log.Fatalf("Error reading stored PIN: %v", err)
	}
	alreadyPaired, err := paired(dataDir)
	if err != nil {
		log.Fatalf("Error reading pairing data: %v", err)
	}
	pin, err := resolvePin(stored, *pinFlag, alreadyPaired)
	if err != nil {
		log.Fatal(err)
	}
	if pin != stored {
		if err := savePin(dataDir, pin); err != nil {
			log.Fatalf("Error storing PIN: %v", err)
		}
	}

	log.Printf("HomeKit pairing data: %s\n", dataDir)
	log.Printf("HomeKit setup PIN: %s\n", formatPin(pin))
	bridge := accessory.NewBridge(accessory.Info{
		Name:         "Litra Bridge",
		SerialNumber: "420-69",
		Manufacturer: "jslay",
		Model:        "litra-ctl",
		Firmware:     "0.0.1",
	})

	beams, err := litra.GetLitraBeams()
	if err != nil {
		log.Fatalf("Error getting connected devices: %v", err)
	}
	if len(beams) == 0 {
		log.Fatalln("No Litra Beams Found. Exiting")
	}

	var beamAcc []*accessory.A
	for _, device := range beams {
		info := accessory.Info{
			Name:         "Litra Beam " + device.SerialNbr,
			SerialNumber: device.SerialNbr,
			Manufacturer: "Logitech",
			Model:        device.ProductStr,
			Firmware:     strconv.Itoa(int(device.ReleaseNbr)),
		}
		ac := accessory.NewLightbulb(info)
		brightness := characteristic.NewBrightness()
		colorTemp := characteristic.NewColorTemperature()
		ac.Lightbulb.AddC(brightness.C)
		ac.Lightbulb.AddC(colorTemp.C)
		beamAcc = append(beamAcc, ac.A)
		log.Printf("Adding accessory for Litra Beam %s\n", device.SerialNbr)

		bindBeam(device, ac.Lightbulb.On, brightness, colorTemp)
		ac.IdentifyFunc = func(*http.Request) {
			go identify(device)
		}
	}

	server, err := hap.NewServer(hap.NewFsStore(dataDir), bridge.A, beamAcc...)
	if err != nil {
		log.Fatalf("Error starting HomeKit server: %v", err)
	}
	server.Pin = pin
	server.Addr = listenAddr(*port)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err = server.ListenAndServe(ctx)
	for _, device := range beams {
		_ = device.Close()
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("HomeKit server stopped: %v", err)
	}
}

func listenAddr(port string) string {
	if port == "" || strings.Contains(port, ":") {
		return port
	}
	return ":" + port
}

func installServiceConflict(pin, port, data string, reset bool) error {
	if pin != "" || port != "" || data != "" || reset {
		return errors.New("-install-service cannot be combined with -pin, -port, -data, or -reset")
	}
	return nil
}

func executablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Abs(exe)
}

func userServiceDir() (string, error) {
	if base := os.Getenv("XDG_CONFIG_HOME"); base != "" {
		return filepath.Join(base, "systemd", "user"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "systemd", "user"), nil
}

func serviceUnit(execPath string) string {
	start := execPath
	if strings.ContainsAny(execPath, " \t") {
		start = strconv.Quote(execPath)
	}
	return fmt.Sprintf(`[Unit]
Description=Litra Beam HomeKit bridge
After=bluetooth.target

[Service]
ExecStart=%s
Restart=on-failure
RestartSec=3

[Install]
WantedBy=default.target
`, start)
}

func installUserService(dir, execPath string, run func(name string, args ...string) error) (string, error) {
	if !filepath.IsAbs(execPath) {
		return "", fmt.Errorf("executable path %q is not absolute", execPath)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "litra-homekit.service")
	if err := os.WriteFile(path, []byte(serviceUnit(execPath)), 0o644); err != nil {
		return "", err
	}
	commands := [][]string{
		{"--user", "daemon-reload"},
		{"--user", "enable", "litra-homekit"},
		{"--user", "restart", "litra-homekit"},
	}
	for _, args := range commands {
		if err := run("systemctl", args...); err != nil {
			return path, err
		}
	}
	return path, nil
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, msg)
	}
	return nil
}

func currentUser() string {
	if name := os.Getenv("USER"); name != "" {
		return name
	}
	account, err := user.Current()
	if err != nil || account.Username == "" {
		return "$USER"
	}
	return account.Username
}

func validPin(pin string) bool {
	if _, trivial := trivialPins[pin]; len(pin) != 8 || trivial {
		return false
	}
	for _, r := range pin {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

var trivialPins = map[string]struct{}{
	"12345678": {},
	"87654321": {},
	"00000000": {},
	"11111111": {},
	"22222222": {},
	"33333333": {},
	"44444444": {},
	"55555555": {},
	"66666666": {},
	"77777777": {},
	"88888888": {},
	"99999999": {},
}

func formatPin(pin string) string {
	return pin[:3] + "-" + pin[3:5] + "-" + pin[5:]
}

func resolvePin(stored, requested string, alreadyPaired bool) (string, error) {
	if requested != "" && !validPin(requested) {
		return "", errors.New("-pin must be eight digits and must not be a trivial code")
	}
	if stored != "" && requested != "" && stored != requested {
		if alreadyPaired {
			return "", errors.New("already paired, -pin does not match the stored PIN. Run with -reset to pair again")
		}
		return requested, nil
	}
	if stored != "" {
		return stored, nil
	}
	if requested != "" {
		return requested, nil
	}
	return generatePin()
}

func generatePin() (string, error) {
	for range 32 {
		var buf [8]byte
		if _, err := rand.Read(buf[:]); err != nil {
			return "", err
		}
		pin := make([]byte, 8)
		for i, b := range buf {
			pin[i] = '0' + (b % 10)
		}
		if candidate := string(pin); validPin(candidate) {
			return candidate, nil
		}
	}
	return "", errors.New("could not generate a PIN")
}

func pinPath(dir string) string {
	return filepath.Join(dir, "pin")
}

func loadPin(dir string) (string, error) {
	b, err := os.ReadFile(pinPath(dir))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	pin := strings.TrimSpace(string(b))
	if pin != "" && !validPin(pin) {
		return "", fmt.Errorf("stored PIN %q is invalid", pin)
	}
	return pin, nil
}

func savePin(dir, pin string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(pinPath(dir), []byte(pin+"\n"), 0o600)
}

func paired(dir string) (bool, error) {
	pairings, err := filepath.Glob(filepath.Join(dir, "*.pairing"))
	if err != nil {
		return false, err
	}
	if len(pairings) > 0 {
		return true, nil
	}
	// Older builds stored the bridge and each controller as *.entity.
	entities, err := filepath.Glob(filepath.Join(dir, "*.entity"))
	if err != nil {
		return false, err
	}
	return len(entities) > 1, nil
}

func defaultDataDir() (string, error) {
	if runtime.GOOS == "windows" {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			base = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(base, "litra-homekit"), nil
	}

	if base := os.Getenv("XDG_DATA_HOME"); base != "" {
		return filepath.Join(base, "litra-homekit"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "litra-homekit"), nil
}

func identify(device *litra.Beam) {
	log.Printf("Identify Litra Beam %s\n", device.SerialNbr)
	for range 5 {
		if err := device.On(); err != nil {
			log.Printf("Identify on: %v\n", err)
		}
		time.Sleep(500 * time.Millisecond)
		if err := device.Off(); err != nil {
			log.Printf("Identify off: %v\n", err)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// bindBeam serves live light state on HomeKit reads and applies writes to the Beam.
func bindBeam(device *litra.Beam, on *characteristic.On, brightness *characteristic.Brightness, colorTemp *characteristic.ColorTemperature) {
	var mu sync.Mutex
	var cached litra.State
	var cachedAt time.Time

	load := func() (litra.State, bool) {
		mu.Lock()
		if !cachedAt.IsZero() && time.Since(cachedAt) < 200*time.Millisecond {
			state := cached
			mu.Unlock()
			return state, true
		}
		mu.Unlock()

		state, err := device.State()
		if err != nil {
			log.Printf("Error reading state for %s: %v\n", device.SerialNbr, err)
			mu.Lock()
			defer mu.Unlock()
			if cachedAt.IsZero() {
				return litra.State{}, false
			}
			return cached, true
		}

		mu.Lock()
		cached = state
		cachedAt = time.Now()
		mu.Unlock()
		return state, true
	}

	invalidate := func() {
		mu.Lock()
		cachedAt = time.Time{}
		mu.Unlock()
	}

	publish := func(state litra.State) {
		on.SetValue(state.On)
		if err := brightness.SetValue(state.Brightness); err != nil {
			log.Printf("Error publishing brightness for %s: %v\n", device.SerialNbr, err)
		}
		if state.Temperature > 0 {
			if err := colorTemp.SetValue(1000000 / state.Temperature); err != nil {
				log.Printf("Error publishing color temperature for %s: %v\n", device.SerialNbr, err)
			}
		}
	}

	on.ValueRequestFunc = func(*http.Request) (interface{}, int) {
		state, ok := load()
		if !ok {
			return on.Value(), 0
		}
		publish(state)
		return state.On, 0
	}
	on.OnSetRemoteValue(func(value bool) error {
		var err error
		if value {
			err = device.On()
		} else {
			err = device.Off()
		}
		if err != nil {
			log.Printf("Error setting power (%t): %v\n", value, err)
			return err
		}
		invalidate()
		return nil
	})

	brightness.ValueRequestFunc = func(*http.Request) (interface{}, int) {
		state, ok := load()
		if !ok {
			return brightness.Value(), 0
		}
		publish(state)
		return state.Brightness, 0
	}
	brightness.OnSetRemoteValue(func(value int) error {
		if err := device.SetBrightness(value); err != nil {
			log.Printf("Error setting brightness (%d): %v\n", value, err)
			return err
		}
		invalidate()
		return nil
	})

	colorTemp.ValueRequestFunc = func(*http.Request) (interface{}, int) {
		state, ok := load()
		if !ok || state.Temperature <= 0 {
			return colorTemp.Value(), 0
		}
		publish(state)
		return 1000000 / state.Temperature, 0
	}
	colorTemp.OnSetRemoteValue(func(value int) error {
		if value <= 0 {
			return fmt.Errorf("color temperature %d", value)
		}
		if err := device.SetTemperature(1000000 / value); err != nil {
			log.Printf("Error setting color temperature (%d): %v\n", value, err)
			return err
		}
		invalidate()
		return nil
	})

	if state, ok := load(); ok {
		publish(state)
	}
}
