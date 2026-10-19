package main

import "testing"

func unsetOptions() cliOptions {
	return cliOptions{
		brightness:         -1,
		temperature:        -1,
		temperaturePercent: -1,
	}
}

func TestValidateOptionsAcceptsCombinedSettings(t *testing.T) {
	cases := []cliOptions{
		unsetOptions(),
		func() cliOptions {
			opts := unsetOptions()
			opts.on = true
			opts.brightness = 100
			opts.temperature = 3900
			opts.status = true
			return opts
		}(),
		func() cliOptions {
			opts := unsetOptions()
			opts.brightness = 0
			opts.temperature = 2700
			return opts
		}(),
		func() cliOptions {
			opts := unsetOptions()
			opts.off = true
			opts.brightness = 100
			opts.temperaturePercent = 0
			return opts
		}(),
		func() cliOptions {
			opts := unsetOptions()
			opts.buildUdev = true
			opts.user = "jslay"
			return opts
		}(),
	}
	for _, opts := range cases {
		if err := validateOptions(opts); err != nil {
			t.Fatal(err)
		}
	}
}

func TestValidateOptionsRejectsConflicts(t *testing.T) {
	tests := []struct {
		name string
		edit func(*cliOptions)
	}{
		{"on and off", func(o *cliOptions) { o.on, o.off = true, true }},
		{"both temperatures", func(o *cliOptions) { o.temperature, o.temperaturePercent = 2700, 50 }},
		{"temperature too low", func(o *cliOptions) { o.temperature = 2699 }},
		{"temperature too high", func(o *cliOptions) { o.temperature = 6501 }},
		{"temperature percent too high", func(o *cliOptions) { o.temperaturePercent = 101 }},
		{"brightness too high", func(o *cliOptions) { o.brightness = 101 }},
		{"user without udev", func(o *cliOptions) { o.user = "jslay" }},
		{"udev with power", func(o *cliOptions) { o.buildUdev, o.on = true, true }},
		{"list and paths", func(o *cliOptions) { o.list, o.pathsOnly = true, true }},
		{"list with status", func(o *cliOptions) { o.list, o.status = true, true }},
		{"paths with device", func(o *cliOptions) { o.pathsOnly, o.device = true, "abc" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			opts := unsetOptions()
			test.edit(&opts)
			if err := validateOptions(opts); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}
