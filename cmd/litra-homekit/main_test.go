package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidPin(t *testing.T) {
	if !validPin("00102003") {
		t.Fatal("expected 00102003 to be valid")
	}
	for _, pin := range []string{"001-0203", "0010200", "0010200a", "", "12345678", "00000000"} {
		if validPin(pin) {
			t.Fatalf("expected %q to be invalid", pin)
		}
	}
}

func TestResolvePin(t *testing.T) {
	got, err := resolvePin("", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if !validPin(got) {
		t.Fatalf("generated pin %q is invalid", got)
	}
	if formatPin("00102003") != "001-02-003" {
		t.Fatalf("formatPin = %s", formatPin("00102003"))
	}

	got, err = resolvePin("11112222", "", true)
	if err != nil || got != "11112222" {
		t.Fatalf("stored pin = %q, %v", got, err)
	}
	got, err = resolvePin("11112222", "11112222", true)
	if err != nil || got != "11112222" {
		t.Fatalf("matching pin = %q, %v", got, err)
	}
	if _, err = resolvePin("11112222", "33334444", true); err == nil {
		t.Fatal("expected a paired pin change to fail")
	}
	got, err = resolvePin("11112222", "33334444", false)
	if err != nil || got != "33334444" {
		t.Fatalf("unpaired pin change = %q, %v", got, err)
	}
}

func TestPaired(t *testing.T) {
	dir := t.TempDir()
	ok, err := paired(dir)
	if err != nil || ok {
		t.Fatalf("empty dir paired = %v, %v", ok, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "aa.entity"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	ok, err = paired(dir)
	if err != nil || ok {
		t.Fatalf("one entity paired = %v, %v", ok, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bb.entity"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	ok, err = paired(dir)
	if err != nil || !ok {
		t.Fatalf("two entities paired = %v, %v", ok, err)
	}
}

func TestPairedHap(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "aa.pairing"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	ok, err := paired(dir)
	if err != nil || !ok {
		t.Fatalf("pairing file paired = %v, %v", ok, err)
	}
}

func TestListenAddr(t *testing.T) {
	if listenAddr("") != "" || listenAddr("51826") != ":51826" || listenAddr(":51826") != ":51826" {
		t.Fatalf("listenAddr = %q %q %q", listenAddr(""), listenAddr("51826"), listenAddr(":51826"))
	}
}

func TestInstallServiceConflict(t *testing.T) {
	if err := installServiceConflict("", "", "", false); err != nil {
		t.Fatal(err)
	}
	for _, call := range []struct {
		pin, port, data string
		reset           bool
	}{
		{pin: "00102003"},
		{port: "51826"},
		{data: "/tmp/litra"},
		{reset: true},
	} {
		if err := installServiceConflict(call.pin, call.port, call.data, call.reset); err == nil {
			t.Fatalf("expected conflict for %+v", call)
		}
	}
}

func TestInstallUserService(t *testing.T) {
	dir := t.TempDir()
	var got []string
	path, err := installUserService(dir, "/home/user/.local/bin/litra-homekit", func(name string, args ...string) error {
		got = append(got, name+" "+strings.Join(args, " "))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "ExecStart=/home/user/.local/bin/litra-homekit\n") {
		t.Fatalf("unit:\n%s", body)
	}
	want := []string{
		"systemctl --user daemon-reload",
		"systemctl --user enable litra-homekit",
		"systemctl --user restart litra-homekit",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("commands = %v", got)
	}
	if _, err := installUserService(dir, "litra-homekit", func(string, ...string) error { return nil }); err == nil {
		t.Fatal("expected a relative executable path to fail")
	}
}

func TestUserServiceDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/litra-config")
	dir, err := userServiceDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join("/tmp/litra-config", "systemd", "user") {
		t.Fatalf("userServiceDir = %s", dir)
	}
}

func TestDefaultDataDir(t *testing.T) {
	dir, err := defaultDataDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir == "" {
		t.Fatal("expected a data directory")
	}
}
