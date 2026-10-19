# go-litra-beam
A simple Go package for controlling Logitech Litra Beam via USB or Bluetooth.

## litra-ctl
Check the
[Releases](https://github.com/jslay88/go-litra-beam/releases)
page for pre-built binary downloads.

On Linux you may need udev rules, or to run as root, if you get permission denied errors.

```
Usage of litra-ctl:
  -brightness int
        Set the brightness level (0-100) (default -1)
  -device string
        The device to control via serial number. If not defined, will default to the first light discovered
  -list
        Lists the discovered lights by their serial numbers and their paths, then exit
  -off
        Turn the Litra Beam off
  -on
        Turn the Litra Beam on
  -paths-only
        Outputs just the device paths and exits
  -status
        Print power, brightness, and color temperature
  -temperature int
        Set the color temperature using a Kelvin value (2700-6500) (default -1)
  -temperature-percent int
        Set the color temperature by percentage (0-100) (default -1)
```

Linux builds also register:

```
  -build-udev-rules
        Build and install udev rules, then exit
  -user string
        User to install udev rules for, usually $USER. Only used with -build-udev-rules
```

Windows builds do not register those two flags, so they are absent from help.

### Examples
```
litra-ctl -list    # List discovered lights by serial number and exit
litra-ctl -status  # Print power, brightness, and color temperature
litra-ctl -on      # Turn on the light
litra-ctl -off   # Turn off the light
litra-ctl -on -brightness 100   # Turn on the light and set full brightness
litra-ctl -brightness 50        # Set the brightness to 50% without changing the on/off state.
litra-ctl -temperature 3900     # Set the temperature to 3900K without changing the on/off state.
litra-ctl -on -brightness 100 -temperature-percent 100   # Turn on light, set full brightness, set temperature to 6500K
litra-ctl -device de:ad:be:ef:4f:a9 -on -brightness 100  # Turn on light with serial number 'de:ad:be:ef:4f:a9' and set full brightness
sudo litra-ctl -build-udev-rules -user $USER   # Build udev rules for current user. Requires root privileges.
```

Brightness is applied, then color temperature, then power. `-status` prints the result. The order of those flags on the command line does not matter. `-on` with `-off`, both temperature flags, and `-list` or `-paths-only` mixed with a change are rejected.

## litra-homekit

`litra-homekit` is the same light, exposed as a HomeKit bridge. It has to keep running. The Home app pairs to that process, and the pairing keys are files on disk. If those files move or get deleted, the iPhone has to pair again. A data directory written by an older build is migrated the first time this version starts.

```
litra-homekit
litra-homekit -pin 49281357
litra-homekit -data /var/lib/litra-homekit
litra-homekit -port 51826
litra-homekit -reset
```

The first launch writes a setup PIN into the data directory and logs it as `XXX-XX-XXX`. Later launches reuse that file. `-pin` sets the code before anything is paired. After a phone is paired, a different `-pin` is rejected. `-port` empty lets the OS pick a port and mDNS advertises it.

`-reset` deletes the data directory and exits. Remove the bridge in the Home app, then start `litra-homekit` again. The next launch is a new accessory with a new PIN.

Pairing data defaults to:

- Linux: `$XDG_DATA_HOME/litra-homekit` or `~/.local/share/litra-homekit`
- Windows: `%LOCALAPPDATA%\litra-homekit`

Release builds include `litra-homekit` next to `litra-ctl`. Put the Linux binary at `~/.local/bin/litra-homekit`.

In the Home app: Add Accessory, More options, Litra Bridge, then the PIN.

The bridge exits if it cannot see a Beam at startup, so the light needs to be connected first.

### Linux

Use a user service. The udev rule from `litra-ctl -build-udev-rules` grants your user the hidraw device. A system unit runs as somebody else and does not get that access.

Install the binary at `~/.local/bin/litra-homekit`, then:

```
litra-homekit -install-service
```

That writes `~/.config/systemd/user/litra-homekit.service` with `ExecStart` set to the binary you ran, enables the user service, and starts it. `-install-service` cannot be combined with `-pin`, `-port`, `-data`, or `-reset`. `journalctl --user -u litra-homekit` shows the data directory and the setup PIN.

The service stops when your login session ends. To leave it running after logout:

```
sudo loginctl enable-linger $USER
```

`packaging/systemd/user/litra-homekit.service` is the same unit with `ExecStart` pointed at `~/.local/bin/litra-homekit`, for copying by hand.

### Windows

The binary is not a Windows service. `sc.exe create` will start it and then kill it, because it does not speak the service control protocol.

Register a logon task instead. From the repo, in PowerShell:

```
powershell -ExecutionPolicy Bypass -File packaging/windows/install-homekit-task.ps1 -ExePath C:\path\to\litra-homekit.exe
```

The task is named `Litra HomeKit`. It starts at logon, restarts up to three times if it dies, and does not stop on battery. Pair from the Home app on the same network. The phone does not need to be on the Windows machine's user account, it just has to reach that process.

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
        VendorId  = 0x46d
        ProductId = 0xb901

        Handler                = 0x11
        Header1                = 0xff
        Header2                = 0x04
        PropertyOnOffGet       = 0x0c
        PropertyOnOff          = 0x1c
        PropertyBrightnessGet  = 0x3c
        PropertyBrightness     = 0x4c
        PropertyTemperatureGet = 0x8c
        PropertyTemperature    = 0x9c

        MinBrightnessPercent = 0
        MaxBrightnessPercent = 100
        MinTemperatureK      = 2700
        MaxTemperatureK      = 6500
)

TYPES

type State struct {
        On                 bool
        Brightness         int
        BrightnessLumens   int
        Temperature        int
        TemperaturePercent int
}
    State is the illumination state read back from the light. Brightness
    uses the same 0-100 scale as SetBrightness, and Temperature uses the
    same Kelvin scale as SetTemperature.

type Beam struct {
        *hid.DeviceInfo
}
    Beam A Go implementation for the light. The HID connection is opened on the
    first call and kept open for later calls.

func GetLitraBeams() ([]*Beam, error)
    GetLitraBeams Returns a slice of *Beam for the discovered lights

func NewBeam(serialNumber string) (*Beam, error)
    NewBeam Get a *Beam via the device serial number

func (b *Beam) BrightnessLumens() (int, error)
    BrightnessLumens reads the current brightness in lumens.

func (b *Beam) Close() error
    Close closes the HID connection. The next call opens it again.

func (b *Beam) IsOn() (bool, error)
    IsOn reports whether the light is on.

func (b *Beam) Off() error

func (b *Beam) On() error

func (b *Beam) ReadProperty(property byte) ([]byte, error)
    ReadProperty sends a get-function report and returns the matching response.
    The response includes the report ID. Payload bytes start at index 4.

func (b *Beam) SetBrightness(brightness int) error
    SetBrightness Set the brightness of the light using a percentage, 0-100

func (b *Beam) SetTemperature(temperature int) error
    SetTemperature Set the temperature of the light using a Kelvin value
    2700-6500. Valid values are only in increments of 100 (2700, 2800, 2900,
    etc.), and it will round to the nearest.

func (b *Beam) SetTemperaturePercentage(temperature int) error
    SetTemperaturePercentage Set the temperature of the light using a percentage
    0-100

func (b *Beam) State() (State, error)
    State reads power, brightness, and color temperature.

func (b *Beam) Temperature() (int, error)
    Temperature reads the current color temperature in Kelvin.

func (b *Beam) WriteProperty(property byte, data []byte) (int, error)
    WriteProperty Write a property and values to the Light
```