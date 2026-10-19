package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"text/template"

	"github.com/jslay88/go-litra-beam"
)

type cliOptions struct {
	list               bool
	device             string
	on                 bool
	off                bool
	brightness         int
	temperature        int
	temperaturePercent int
	pathsOnly          bool
	status             bool
	buildUdev          bool
	user               string
}

const (
	udevTemplate = `ACTION=="add", SUBSYSTEMS=="hidraw", RUN+="/bin/sh -c 'for i in $({{ .ExecutablePath }} -paths-only); do setfacl -m u:{{ .User }}:rw $i; done'", MODE="0660"`
)

func main() {
	opts := cliOptions{
		brightness:         -1,
		temperature:        -1,
		temperaturePercent: -1,
	}
	flag.BoolVar(&opts.list, "list", false, "Lists the discovered lights by their serial numbers and their paths, then exit")
	flag.StringVar(&opts.device, "device", "", "The device to control via serial number. If not defined, will default to the first light discovered")
	flag.BoolVar(&opts.on, "on", false, "Turn the Litra Beam on")
	flag.BoolVar(&opts.off, "off", false, "Turn the Litra Beam off")
	flag.IntVar(&opts.brightness, "brightness", -1, "Set the brightness level (0-100)")
	flag.IntVar(&opts.temperature, "temperature", -1, "Set the color temperature using a Kelvin value (2700-6500)")
	flag.IntVar(&opts.temperaturePercent, "temperature-percent", -1, "Set the color temperature by percentage (0-100)")
	flag.BoolVar(&opts.pathsOnly, "paths-only", false, "Outputs just the device paths and exits")
	flag.BoolVar(&opts.status, "status", false, "Print power, brightness, and color temperature")
	if runtime.GOOS == "linux" {
		flag.BoolVar(&opts.buildUdev, "build-udev-rules", false, "Build and install udev rules, then exit")
		flag.StringVar(&opts.user, "user", "", "User to install udev rules for, usually $USER. Only used with -build-udev-rules")
	}
	flag.Parse()

	if err := validateOptions(opts); err != nil {
		log.Fatal(err)
	}

	if opts.buildUdev {
		buildUdevRules(opts.user)
		os.Exit(0)
		return
	}

	if !opts.list && !opts.pathsOnly && opts.device == "" && !opts.hasSetting() {
		fmt.Println("No options provided. Discovering Litra Beams...")
		opts.list = true
	}

	var d *litra.Beam
	if opts.list || opts.pathsOnly || opts.device == "" {
		devices, err := litra.GetLitraBeams()
		if err != nil {
			log.Fatalf("Error getting connected devices: %v", err)
		}
		if len(devices) == 0 {
			log.Fatalln("No Litra Beam devices found.")
		}
		if opts.list || opts.pathsOnly {
			if opts.list {
				printDeviceList(devices)
			} else {
				printPathsOnly(devices)
			}
			os.Exit(0)
			return
		}
		d = devices[0]
	} else {
		var err error
		d, err = litra.NewBeam(opts.device)
		if err != nil {
			log.Fatalf("Error discovering device by serial number. Serial Number: %s, Error: %v", opts.device, err)
		}
	}

	if opts.brightness != -1 {
		if err := d.SetBrightness(opts.brightness); err != nil {
			log.Fatalf("Error setting brightness: %v", err)
		}
	}

	if opts.temperature != -1 {
		if err := d.SetTemperature(opts.temperature); err != nil {
			log.Fatalf("Error setting temperature: %v", err)
		}
	} else if opts.temperaturePercent != -1 {
		if err := d.SetTemperaturePercentage(opts.temperaturePercent); err != nil {
			log.Fatalf("Error setting temperature by percentage: %v", err)
		}
	}

	if opts.on {
		if err := d.On(); err != nil {
			log.Fatalf("Error turning on device: %v", err)
		}
	} else if opts.off {
		if err := d.Off(); err != nil {
			log.Fatalf("Error turning off device: %v", err)
		}
	}

	if opts.status {
		printStatus(d)
	}
	_ = d.Close()
}

func (o cliOptions) hasSetting() bool {
	return o.on || o.off || o.brightness != -1 || o.temperature != -1 || o.temperaturePercent != -1 || o.status
}

func validateOptions(o cliOptions) error {
	if o.on && o.off {
		return errors.New("-on and -off cannot be used together")
	}
	if o.temperature != -1 && o.temperaturePercent != -1 {
		return errors.New("-temperature and -temperature-percent cannot be used together")
	}
	if o.temperature != -1 && (o.temperature < litra.MinTemperatureK || o.temperature > litra.MaxTemperatureK) {
		return fmt.Errorf("-temperature must be from %d to %d, got %d", litra.MinTemperatureK, litra.MaxTemperatureK, o.temperature)
	}
	if o.temperaturePercent != -1 && (o.temperaturePercent < litra.MinBrightnessPercent || o.temperaturePercent > litra.MaxBrightnessPercent) {
		return fmt.Errorf("-temperature-percent must be from %d to %d, got %d", litra.MinBrightnessPercent, litra.MaxBrightnessPercent, o.temperaturePercent)
	}
	if o.brightness != -1 && (o.brightness < litra.MinBrightnessPercent || o.brightness > litra.MaxBrightnessPercent) {
		return fmt.Errorf("-brightness must be from %d to %d, got %d", litra.MinBrightnessPercent, litra.MaxBrightnessPercent, o.brightness)
	}
	if o.user != "" && !o.buildUdev {
		return errors.New("-user can only be used with -build-udev-rules")
	}
	if o.buildUdev && (o.list || o.pathsOnly || o.device != "" || o.hasSetting()) {
		return errors.New("-build-udev-rules cannot be combined with other options")
	}
	if o.list && o.pathsOnly {
		return errors.New("-list and -paths-only cannot be used together")
	}
	if o.list && (o.device != "" || o.hasSetting()) {
		return errors.New("-list cannot be combined with other options")
	}
	if o.pathsOnly && (o.device != "" || o.hasSetting()) {
		return errors.New("-paths-only cannot be combined with other options")
	}
	return nil
}

func buildUdevRules(user string) {
	if runtime.GOOS != "linux" {
		log.Fatalln("udev rule creation is only for Linux.")
	}
	if user == "" {
		log.Fatalln("Cannot detect who to install rules for automatically. Must provide a value for -user")
	}

	tpl, err := template.New("udev").Parse(udevTemplate)
	if err != nil {
		log.Fatalf("Failed to parse udev rule template. %v\n", err)
	}
	out := bytes.Buffer{}
	type data struct {
		ExecutablePath string
		User           string
	}

	var exec string
	exec, err = os.Executable()
	if err != nil {
		log.Fatalf("Failed to get litra-ctl executable path. %v\n", err)
	}

	d := &data{
		ExecutablePath: exec,
		User:           user,
	}
	err = tpl.Execute(&out, d)
	if err != nil {
		log.Fatalf("Failed to render udev template. %v\n", err)
	}

	var oFile *os.File
	filename := "/etc/udev/rules.d/99-litra.rules"
	oFile, err = os.Create(filename)
	if err != nil {
		log.Fatalf("Failed to create udev rules @ %s\n\nError: %v\n", filename, err)
	}
	defer oFile.Close()
	_, err = oFile.Write(append(out.Bytes(), byte('\n')))
	if err != nil {
		log.Fatalf("Failed to write udev rules @ %s\n\nError %v\n", filename, err)
	}
	fmt.Printf("Wrote udev rules @ %s. Rule:\n%s\n", filename, out.String())
}

func printStatus(d *litra.Beam) {
	state, err := d.State()
	if err != nil {
		log.Fatalf("Error reading device state: %v", err)
	}
	power := "off"
	if state.On {
		power = "on"
	}
	fmt.Printf("Serial Number: %s\nPower: %s\nBrightness: %d%% (%d lm)\nTemperature: %dK (%d%%)\n",
		d.SerialNbr, power, state.Brightness, state.BrightnessLumens, state.Temperature, state.TemperaturePercent)
}

func printDeviceList(devices []*litra.Beam) {
	fmt.Println("Discovered Litra Beams:")
	for _, d := range devices {
		fmt.Printf("Serial Number: %s, Path: %s\n", d.SerialNbr, d.Path)
	}
}

func printPathsOnly(devices []*litra.Beam) {
	for _, d := range devices {
		fmt.Println(d.Path)
	}
}
