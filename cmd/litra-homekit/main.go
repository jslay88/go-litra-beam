package main

import (
	"log"
	"strconv"
	"time"

	"github.com/brutella/hc"
	"github.com/brutella/hc/accessory"
	"github.com/brutella/hc/characteristic"
	"github.com/jslay88/go-litra-beam"
)

func main() {
	var err error

	config := hc.Config{Pin: "00102003"}
	bridge := accessory.NewBridge(accessory.Info{
		Name:             "Litra Bridge",
		SerialNumber:     "420-69",
		Manufacturer:     "jslay",
		Model:            "litra-ctl",
		FirmwareRevision: "0.0.1",
		ID:               0,
	})

	var beamAcc []*accessory.Accessory
	var beams []*litra.Beam

	beams, err = litra.GetLitraBeams()
	if err != nil {
		panic(err)
	}

	if len(beams) == 0 {
		log.Fatalln("No Litra Beams Found. Exiting")
	}

	for _, device := range beams {
		info := accessory.Info{
			Name:             "Litra Beam " + device.SerialNbr,
			SerialNumber:     device.SerialNbr,
			Manufacturer:     "Logitech",
			Model:            device.ProductStr,
			FirmwareRevision: strconv.Itoa(int(device.ReleaseNbr)),
			ID:               0,
		}
		ac := accessory.NewLightbulb(info)
		brightness := characteristic.NewBrightness()
		colorTemp := characteristic.NewColorTemperature()
		ac.GetServices()[1].AddCharacteristic(brightness.Characteristic)
		ac.GetServices()[1].AddCharacteristic(colorTemp.Characteristic)
		beamAcc = append(beamAcc, ac.Accessory)
		log.Printf("Adding accessory for Litra Beam %s\n", device.SerialNbr)

		ac.Lightbulb.On.OnValueRemoteUpdate(func(on bool) {
			if on {
				_ = device.On()
			} else {
				_ = device.Off()
			}
		})
		ac.OnIdentify(func() {
			log.Println("Identify Litra Beam")
			for _ = range 5 {
				_ = device.On()
				time.Sleep(500 * time.Millisecond)
				_ = device.Off()
				time.Sleep(500 * time.Millisecond)
			}
		})

		brightness.OnValueRemoteUpdate(func(newBrightness int) {
			if err = device.SetBrightness(newBrightness); err != nil {
				log.Printf("Erro setting brightness (%d): %v\n", newBrightness, err)
			}
		})

		colorTemp.OnValueRemoteUpdate(func(newColor int) {
			if err = device.SetTemperature(1000000 / newColor); err != nil {
				log.Printf("Error setting color temperature (%d): %v\n", newColor, err)
			}
		})
	}

	t, err := hc.NewIPTransport(config, bridge.Accessory, beamAcc...)

	if err != nil {
		panic(nil)
	}
	hc.OnTermination(func() {
		<-t.Stop()
	})
	t.Start()

}
