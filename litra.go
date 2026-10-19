package litra

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/sstallion/go-hid"
)

const (
	VendorId  = 0x46d
	ProductId = 0xb901

	// HID++ 2.0 long report. Header2 is the Illumination feature index (feature ID 0x1990).
	// A function byte is (function index << 4) | software ID 0xC. Get functions sit one
	// index below the matching set function.
	Handler                = 0x11
	Header1                = 0xff
	Header2                = 0x04
	PropertyOnOffGet       = 0x0c
	PropertyOnOff          = 0x1c
	PropertyBrightnessGet  = 0x3c
	PropertyBrightness     = 0x4c
	PropertyTemperatureGet = 0x8c
	PropertyTemperature    = 0x9c

	reportLength = 20

	// Brightness percentages and color temperatures accepted by SetBrightness and SetTemperature.
	MinBrightnessPercent = 0
	MaxBrightnessPercent = 100
	MinTemperatureK      = 2700
	MaxTemperatureK      = 6500

	minBrightnessLumens = 30
	maxBrightnessLumens = 400
)

// State is the illumination state read back from the light.
// Brightness uses the same 0-100 scale as SetBrightness, and Temperature
// uses the same Kelvin scale as SetTemperature.
type State struct {
	On                 bool
	Brightness         int
	BrightnessLumens   int
	Temperature        int
	TemperaturePercent int
}

var errShortResponse = errors.New("litra: short response from device")

// Beam A Go implementation for the light.
// The HID connection is opened on the first call and kept open for later calls.
type Beam struct {
	*hid.DeviceInfo

	mu  sync.Mutex
	dev *hid.Device
}

// NewBeam Get a *Beam via the device serial number
func NewBeam(serialNumber string) (*Beam, error) {
	device, err := hid.Open(VendorId, ProductId, serialNumber)
	if err != nil {
		return nil, err
	}
	defer device.Close()

	var info *hid.DeviceInfo
	info, err = device.GetDeviceInfo()
	if err != nil {
		return nil, err
	}

	return &Beam{
		DeviceInfo: info,
	}, nil
}

func (b *Beam) Off() error {
	_, err := b.WriteProperty(PropertyOnOff, []byte{0x00})
	return err
}

func (b *Beam) On() error {
	_, err := b.WriteProperty(PropertyOnOff, []byte{0x01})
	return err
}

// SetBrightness Set the brightness of the light using a percentage, 0-100
func (b *Beam) SetBrightness(brightness int) error {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, brightnessValue(brightness))
	_, err := b.WriteProperty(PropertyBrightness, buf)
	return err
}

// SetTemperature Set the temperature of the light using a Kelvin value 2700-6500.
// Valid values are only in increments of 100 (2700, 2800, 2900, etc.), and it will
// round to the nearest.
func (b *Beam) SetTemperature(temperature int) error {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, temperatureValue(temperature))
	_, err := b.WriteProperty(PropertyTemperature, buf)
	return err
}

// SetTemperaturePercentage Set the temperature of the light using a percentage 0-100
func (b *Beam) SetTemperaturePercentage(temperature int) error {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, temperaturePercentValue(temperature))
	_, err := b.WriteProperty(PropertyTemperature, buf)
	return err
}

func brightnessValue(percent int) uint16 {
	if percent < MinBrightnessPercent {
		percent = MinBrightnessPercent
	} else if percent > MaxBrightnessPercent {
		percent = MaxBrightnessPercent
	}
	span := float64(maxBrightnessLumens - minBrightnessLumens)
	return uint16(math.Round(float64(minBrightnessLumens) + (float64(percent)/100.0)*span))
}

func temperatureValue(kelvin int) uint16 {
	if kelvin < MinTemperatureK {
		kelvin = MinTemperatureK
	} else if kelvin > MaxTemperatureK {
		kelvin = MaxTemperatureK
	}
	return uint16(math.Round(float64(kelvin)/100.0) * 100)
}

func temperaturePercentValue(percent int) uint16 {
	if percent < MinBrightnessPercent {
		percent = MinBrightnessPercent
	} else if percent > MaxBrightnessPercent {
		percent = MaxBrightnessPercent
	}
	span := float64(MaxTemperatureK - MinTemperatureK)
	kelvin := float64(MinTemperatureK) + (float64(percent)/100.0)*span
	return uint16(math.Round(kelvin/100.0) * 100)
}

// Close closes the HID connection. The next call opens it again.
func (b *Beam) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closeLocked()
}

func (b *Beam) closeLocked() error {
	if b.dev == nil {
		return nil
	}
	err := b.dev.Close()
	b.dev = nil
	return err
}

func (b *Beam) withDevice(fn func(*hid.Device) error) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.dev == nil {
		dev, err := hid.Open(VendorId, ProductId, b.SerialNbr)
		if err != nil {
			return err
		}
		b.dev = dev
	}
	if err := fn(b.dev); err != nil {
		if !errors.Is(err, errShortResponse) && !isResponseError(err) {
			_ = b.closeLocked()
		}
		return err
	}
	return nil
}

// WriteProperty Write a property and values to the Light
func (b *Beam) WriteProperty(property byte, data []byte) (int, error) {
	var n int
	err := b.withDevice(func(device *hid.Device) error {
		buf := make([]byte, reportLength)
		buf[0] = Handler
		buf[1] = Header1
		buf[2] = Header2
		buf[3] = property
		copy(buf[4:], data)
		var err error
		n, err = device.Write(buf)
		return err
	})
	return n, err
}

// ReadProperty sends a get-function report and returns the matching response.
// The response includes the report ID. Payload bytes start at index 4.
func (b *Beam) ReadProperty(property byte) ([]byte, error) {
	var resp []byte
	err := b.withDevice(func(device *hid.Device) error {
		var err error
		resp, err = readReport(device, property)
		return err
	})
	return resp, err
}

// IsOn reports whether the light is on.
func (b *Beam) IsOn() (bool, error) {
	resp, err := b.ReadProperty(PropertyOnOffGet)
	if err != nil {
		return false, err
	}
	if len(resp) < 5 {
		return false, errShortResponse
	}
	return resp[4] != 0, nil
}

// BrightnessLumens reads the current brightness in lumens.
func (b *Beam) BrightnessLumens() (int, error) {
	return b.readUint16(PropertyBrightnessGet)
}

// Temperature reads the current color temperature in Kelvin.
func (b *Beam) Temperature() (int, error) {
	return b.readUint16(PropertyTemperatureGet)
}

// State reads power, brightness, and color temperature.
func (b *Beam) State() (State, error) {
	var state State
	err := b.withDevice(func(device *hid.Device) error {
		power, err := readReport(device, PropertyOnOffGet)
		if err != nil {
			return err
		}
		brightness, err := readReport(device, PropertyBrightnessGet)
		if err != nil {
			return err
		}
		temperature, err := readReport(device, PropertyTemperatureGet)
		if err != nil {
			return err
		}
		state, err = parseState(power, brightness, temperature)
		return err
	})
	return state, err
}

func (b *Beam) readUint16(property byte) (int, error) {
	resp, err := b.ReadProperty(property)
	if err != nil {
		return 0, err
	}
	if len(resp) < 6 {
		return 0, errShortResponse
	}
	return int(binary.BigEndian.Uint16(resp[4:6])), nil
}

func readReport(device *hid.Device, property byte) ([]byte, error) {
	buf := make([]byte, reportLength)
	buf[0] = Handler
	buf[1] = Header1
	buf[2] = Header2
	buf[3] = property
	if _, err := device.Write(buf); err != nil {
		return nil, err
	}

	resp := make([]byte, reportLength)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		n, err := device.ReadWithTimeout(resp, 200*time.Millisecond)
		if err != nil {
			if errors.Is(err, hid.ErrTimeout) || interrupted(err) {
				continue
			}
			return nil, err
		}
		if n < 4 || resp[0] != Handler {
			continue
		}
		// HID++ error responses insert 0xFF in the feature-index position.
		if resp[2] == 0xff && n >= 6 {
			return nil, &responseError{fmt.Sprintf("litra: device rejected request (hid++ error 0x%02x)", resp[5])}
		}
		if resp[1] == Header1 && resp[2] == Header2 && resp[3] == property {
			out := make([]byte, n)
			copy(out, resp[:n])
			return out, nil
		}
	}
	return nil, &responseError{fmt.Sprintf("litra: no response for function 0x%02x", property)}
}

type responseError struct {
	msg string
}

func (e *responseError) Error() string {
	return e.msg
}

func isResponseError(err error) bool {
	var target *responseError
	return errors.As(err, &target)
}

func parseState(power, brightness, temperature []byte) (State, error) {
	if len(power) < 5 || len(brightness) < 6 || len(temperature) < 6 {
		return State{}, errShortResponse
	}
	lumens := int(binary.BigEndian.Uint16(brightness[4:6]))
	kelvin := int(binary.BigEndian.Uint16(temperature[4:6]))
	return State{
		On:                 power[4] != 0,
		BrightnessLumens:   lumens,
		Brightness:         percentBetween(lumens, minBrightnessLumens, maxBrightnessLumens),
		Temperature:        kelvin,
		TemperaturePercent: percentBetween(kelvin, MinTemperatureK, MaxTemperatureK),
	}, nil
}

func interrupted(err error) bool {
	return errors.Is(err, syscall.EINTR) || strings.EqualFold(err.Error(), syscall.EINTR.Error())
}

func percentBetween(value, min, max int) int {
	if max <= min || value <= min {
		return 0
	}
	if value >= max {
		return 100
	}
	return int(math.Round((float64(value-min) / float64(max-min)) * 100))
}

// GetLitraBeams Returns a slice of *Beam for the discovered lights
func GetLitraBeams() ([]*Beam, error) {
	var lights []*Beam
	var found []string
	err := hid.Enumerate(VendorId, ProductId, func(info *hid.DeviceInfo) error {
		if slices.Contains(found, info.SerialNbr) {
			return nil
		}
		found = append(found, info.SerialNbr)
		lights = append(lights, &Beam{
			DeviceInfo: info,
		})
		return nil
	})
	return lights, err
}
