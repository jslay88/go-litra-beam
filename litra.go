package litra

import (
	"encoding/binary"
	"math"
	"slices"

	"github.com/sstallion/go-hid"
)

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

// Beam A Go implementation for the light
type Beam struct {
	*hid.DeviceInfo
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
	if brightness < 0 {
		brightness = 0
	} else if brightness > 100 {
		brightness = 100
	}
	minValue, maxValue := float64(30), float64(400)

	value := uint16(math.Round(minValue + (float64(brightness)/100.0)*(maxValue-minValue)))

	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, value)
	_, err := b.WriteProperty(PropertyBrightness, buf)
	return err
}

// SetTemperature Set the temperature of the light using a Kelvin value 2700-6500.
// Valid values are only in increments of 100 (2700, 2800, 2900, etc.), and it will
// round to the nearest.
func (b *Beam) SetTemperature(temperature int) error {
	if temperature < 2700 {
		temperature = 2700
	} else if temperature > 6500 {
		temperature = 6500
	}

	value := uint16(math.Round(float64(temperature)/100.0) * 100)

	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, value)
	_, err := b.WriteProperty(PropertyTemperature, buf)
	return err
}

// SetTemperaturePercentage Set the temperature of the light using a percentage 0-100
func (b *Beam) SetTemperaturePercentage(temperature int) error {
	if temperature < 0 {
		temperature = 0
	} else if temperature > 100 {
		temperature = 100
	}

	minValue, maxValue := float64(2700), float64(6500)
	intermediateValue := minValue + (float64(temperature)/100.0)*(maxValue-minValue)
	value := uint16(math.Round(intermediateValue/100.0) * 100)

	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, value)
	_, err := b.WriteProperty(PropertyTemperature, buf)
	return err
}

// WriteProperty Write a property and values to the Light
func (b *Beam) WriteProperty(property byte, data []byte) (int, error) {
	device, err := hid.Open(VendorId, ProductId, b.SerialNbr)
	if err != nil {
		return 0, err
	}
	defer device.Close()
	buf := []byte{Handler, Header1, Header2, property}
	buf = append(buf, data...)
	return device.Write(buf)
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
