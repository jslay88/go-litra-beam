package litra

import (
	"encoding/binary"
	"errors"
	"testing"
)

func TestParseState(t *testing.T) {
	state, err := parseState(
		[]byte{0x11, 0xff, 0x04, 0x0c, 0x01},
		[]byte{0x11, 0xff, 0x04, 0x3c, 0x01, 0x21},
		[]byte{0x11, 0xff, 0x04, 0x8c, 0x0a, 0x8c},
	)
	if err != nil {
		t.Fatalf("parseState: %v", err)
	}
	if !state.On {
		t.Fatal("expected light on")
	}
	if state.BrightnessLumens != 289 || state.Brightness != 70 {
		t.Fatalf("brightness = %d%% (%d lm), want 70%% (289 lm)", state.Brightness, state.BrightnessLumens)
	}
	if state.Temperature != 2700 || state.TemperaturePercent != 0 {
		t.Fatalf("temperature = %dK (%d%%), want 2700K (0%%)", state.Temperature, state.TemperaturePercent)
	}
}

func TestParseStateOff(t *testing.T) {
	state, err := parseState(
		[]byte{0x11, 0xff, 0x04, 0x0c, 0x00},
		[]byte{0x11, 0xff, 0x04, 0x3c, 0x01, 0x90},
		[]byte{0x11, 0xff, 0x04, 0x8c, 0x19, 0x64},
	)
	if err != nil {
		t.Fatalf("parseState: %v", err)
	}
	if state.On {
		t.Fatal("expected light off")
	}
	if state.BrightnessLumens != 400 || state.Brightness != 100 {
		t.Fatalf("brightness = %d%% (%d lm), want 100%% (400 lm)", state.Brightness, state.BrightnessLumens)
	}
	if state.Temperature != 6500 || state.TemperaturePercent != 100 {
		t.Fatalf("temperature = %dK (%d%%), want 6500K (100%%)", state.Temperature, state.TemperaturePercent)
	}
}

func TestParseStateShortResponse(t *testing.T) {
	_, err := parseState([]byte{0x11}, []byte{0x11}, []byte{0x11})
	if err == nil {
		t.Fatal("expected short response error")
	}
}

func TestInterrupted(t *testing.T) {
	if !interrupted(errors.New("Interrupted system call")) {
		t.Fatal("expected hidapi EINTR text to be retryable")
	}
	if interrupted(errors.New("device not found")) {
		t.Fatal("unexpected retry for an unrelated error")
	}
}

func TestPercentBetween(t *testing.T) {
	if got := percentBetween(30, 30, 400); got != 0 {
		t.Fatalf("min brightness percent = %d, want 0", got)
	}
	if got := percentBetween(4600, 2700, 6500); got != 50 {
		t.Fatalf("mid temperature percent = %d, want 50", got)
	}
	if got := percentBetween(0, 30, 400); got != 0 {
		t.Fatalf("below min percent = %d, want 0", got)
	}
	if got := percentBetween(7000, 2700, 6500); got != 100 {
		t.Fatalf("above max percent = %d, want 100", got)
	}
}

func TestBrightnessValue(t *testing.T) {
	tests := []struct {
		percent int
		lumens  uint16
	}{
		{0, 30},
		{70, 289},
		{100, 400},
		{-10, 30},
		{150, 400},
	}
	for _, test := range tests {
		got := brightnessValue(test.percent)
		if got != test.lumens {
			t.Fatalf("brightnessValue(%d) = %d, want %d", test.percent, got, test.lumens)
		}
		var buf [2]byte
		binary.BigEndian.PutUint16(buf[:], got)
		if parsed := percentBetween(int(got), minBrightnessLumens, maxBrightnessLumens); parsed != clampPercent(test.percent) {
			t.Fatalf("brightness %d lm from %d%% reads back as %d%%", got, test.percent, parsed)
		}
	}
	if brightnessValue(70) != 0x0121 {
		t.Fatalf("70%% wire value = %#x, want 0x0121", brightnessValue(70))
	}
}

func TestTemperatureValue(t *testing.T) {
	tests := []struct {
		kelvin int
		want   uint16
	}{
		{2700, 2700},
		{2749, 2700},
		{2750, 2800},
		{6500, 6500},
		{2000, 2700},
		{9000, 6500},
	}
	for _, test := range tests {
		if got := temperatureValue(test.kelvin); got != test.want {
			t.Fatalf("temperatureValue(%d) = %d, want %d", test.kelvin, got, test.want)
		}
	}
	if temperatureValue(2700) != 0x0a8c {
		t.Fatalf("2700K wire value = %#x, want 0x0a8c", temperatureValue(2700))
	}
}

func TestTemperaturePercentValue(t *testing.T) {
	tests := []struct {
		percent int
		kelvin  uint16
	}{
		{0, 2700},
		{50, 4600},
		{100, 6500},
		{-5, 2700},
		{140, 6500},
	}
	for _, test := range tests {
		got := temperaturePercentValue(test.percent)
		if got != test.kelvin {
			t.Fatalf("temperaturePercentValue(%d) = %d, want %d", test.percent, got, test.kelvin)
		}
		if parsed := percentBetween(int(got), MinTemperatureK, MaxTemperatureK); parsed != clampPercent(test.percent) {
			t.Fatalf("%dK from %d%% reads back as %d%%", got, test.percent, parsed)
		}
	}
}

func clampPercent(percent int) int {
	if percent < 0 {
		return 0
	}
	if percent > 100 {
		return 100
	}
	return percent
}
