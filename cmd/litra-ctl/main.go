package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"text/template"

	"github.com/jslay88/go-litra-beam"
)

const (
	udevTemplate = `ACTION=="add", SUBSYSTEMS=="hidraw", RUN+="/bin/sh -c 'for i in $({{ .ExecutablePath }} -paths-only); do setfacl -m u:{{ .User }}:rw $i; done'", MODE="0660"`
)

func main() {
	list := flag.Bool("list", false, "Lists the discovered lights by their serial numbers and their paths, then exit")
	device := flag.String("device", "", "The device to control via serial number. If not defined, will default to the first light discovered")
	on := flag.Bool("on", false, "Turn the LitraBeam on")
	off := flag.Bool("off", false, "Turn the LitraBeam off, ignored if -on is used")
	brightness := flag.Int("brightness", -1, "Set the brightness level (0-100)")
	temperature := flag.Int("temperature", -1, "Set the color temperature using a Kelvin value (2700-6500)")
	temperaturePercent := flag.Int("temperature-percent", -1, "Set the color temperature by percentage (0-100), ignored if -temperature is used")
	pathsOutput := flag.Bool("paths-only", false, "Outputs just the device paths and exits. Useful for udev rules.")
	buildUdev := flag.Bool("build-udev-rules", false, "Build and install udev rules, then exit (for Linux only).")
	user := flag.String("user", "", "User to install udev rules for, usually $USER. Only used with -build-udev-rules (for Linux only).")

	flag.Parse()

	var d *litra.Beam

	if *buildUdev {
		buildUdevRules(*user)
		os.Exit(0)
		return
	}

	if !*list && !*pathsOutput && *device == "" &&
		!*on && !*off && *brightness == -1 &&
		*temperature == -1 && *temperaturePercent == -1 {
		fmt.Println("No options provided. Discovering Litra Beams...")
		*list = true
	}

	if *list || *pathsOutput || *device == "" {
		devices, err := litra.GetLitraBeams()
		if err != nil {
			log.Fatalf("Error getting connected devices: %v", err)
		}
		if len(devices) == 0 {
			log.Fatalln("No Litra Beam devices found.")
		}
		if *list || *pathsOutput {
			if *list {
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
		d, err = litra.NewBeam(*device)
		if err != nil {
			log.Fatalf("Error discovering device by serial number. Serial Number: %s, Error: %v", *device, err)
		}
	}

	if *brightness >= 0 {
		if err := d.SetBrightness(*brightness); err != nil {
			log.Fatalf("Error setting brightness: %v", err)
		}
	}

	if *temperature >= 2700 && *temperature <= 6500 {
		if err := d.SetTemperature(*temperature); err != nil {
			log.Fatalf("Error setting temperature: %v", err)
		}
	} else if *temperaturePercent >= 0 && *temperaturePercent <= 100 {
		if err := d.SetTemperaturePercentage(*temperaturePercent); err != nil {
			log.Fatalf("Error setting temperature by percentage: %v", err)
		}
	}

	if *on {
		if err := d.On(); err != nil {
			log.Fatalf("Error turning on device: %v", err)
		}
	} else if *off {
		if err := d.Off(); err != nil {
			log.Fatalf("Error turning off device: %v", err)
		}
	}
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
