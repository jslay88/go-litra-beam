# go-litra-beam
A simple Go package for controlling Logitech Litra Beam via USB or Bluetooth.

## litra-ctl
Check the
[Releases](https://github.com/jslay88/go-litra-beam/releases)
page for pre-built binary downloads.

NOTE: You may need udev rules or run as root/sudo if you get permission 
denied errors on Linux/macOS.

```
Usage of litra-ctl:
  -brightness int
        Set the brightness level (0-100) (default -1)
  -build-udev-rules
        Build and install udev rules, then exit (for Linux only).
  -device string
        The device to control via serial number. If not defined, will default to the first light discovered
  -list
        Lists the discovered lights by their serial numbers and their paths, then exit
  -off
        Turn the LitraBeam off, ignored if -on is used
  -on
        Turn the LitraBeam on
  -paths-only
        Outputs just the device paths and exits. Useful for udev rules.
  -temperature int
        Set the color temperature using a Kelvin value (2700-6500) (default -1)
  -temperature-percent int
        Set the color temperature by percentage (0-100), ignored if -temperature is used (default -1)
  -user string
        User to install udev rules for, usually $USER. Only used with -build-udev-rules (for Linux only).
```

### Examples
```
litra-ctl -list  # List discovered lights by serial number and exit
litra-ctl -on    # Turn on the light
litra-ctl -off   # Turn off the light
litra-ctl -on -brightness 100   # Turn on the light and set full brightness
litra-ctl -brightness 50        # Set the brightness to 50% without changing the on/off state.
litra-ctl -temperature 3900     # Set the temperature to 3900K without changing the on/off state.
litra-ctl -on -brightness 100 -temperature-percent 100   # Turn on light, set full brightness, set temperature to 6700K
litra-ctl -device de:ad:be:ef:4f:a9 -on -brightness 100  # Turn on light with serial number 'de:ad:be:ef:4f:a9' and set full brightness
sudo litra-ctl -build-udev-rules -user $USER   # Build udev rules for current user. Requires root privileges.
```

## Go Package
Feel free to install and use the `litra` package that [`litra-ctl`](./cmd/litra-ctl/main.go) leverages itself.

```
go get github.com/jslay88/go-litra-beam
```

Then import and use the [`litra`](./litra.go) package.

```go
package main

import (
	"fmt"
	
	"github.com/jslay88/go-litra-beam"
)

func main() {
	devices, _ := litra.GetLitraBeams()
	for _, device := range devices {
		fmt.Printf("Turning on %s\n", device.SerialNbr)
		_ = device.On()
	}
}
```

### go doc
```
package litra // import "github.com/jslay88/go-litra-beam"


CONSTANTS

const (
        VendorId            = 0x46d
        ProductId           = 0xb901
        Handler             = 0x11
        Header1             = 0xff
        Header2             = 0x04
        PropertyOnOff       = 0x1c
        PropertyBrightness  = 0x4c
        PropertyTemperature = 0x9c
)

TYPES

type Beam struct {
        *hid.DeviceInfo
}
    Beam A Go implementation for the light

func GetLitraBeams() ([]*Beam, error)
    GetLitraBeams Returns a slice of *Beam for the discovered lights

func NewBeam(serialNumber string) (*Beam, error)
    NewBeam Get a *Beam via the device serial number

func (b *Beam) Off() error

func (b *Beam) On() error

func (b *Beam) SetBrightness(brightness int) error
    SetBrightness Set the brightness of the light using a percentage, 0-100

func (b *Beam) SetTemperature(temperature int) error
    SetTemperature Set the temperature of the light using a Kelvin value
    2700-6500. Valid values are only in increments of 100 (2700, 2800, 2900,
    etc.), and it will round to the nearest.

func (b *Beam) SetTemperaturePercentage(temperature int) error
    SetTemperaturePercentage Set the temperature of the light using a percentage
    0-100

func (b *Beam) WriteProperty(property byte, data []byte) (int, error)
    WriteProperty Write a property and values to the Light
```