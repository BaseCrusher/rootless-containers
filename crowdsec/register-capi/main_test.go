package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsRegistered(t *testing.T) {
	dir := t.TempDir()

	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}

	if isRegistered(filepath.Join(dir, "missing.yaml")) {
		t.Fatal("missing file: want not registered")
	}
	if isRegistered(write("empty.yaml", "")) {
		t.Fatal("empty file: want not registered")
	}
	if isRegistered(write("nologin.yaml", "url: https://api.crowdsec.net/\n")) {
		t.Fatal("no login: want not registered")
	}
	if isRegistered(write("blank.yaml", "login:\npassword: x\n")) {
		t.Fatal("blank login: want not registered")
	}
	if !isRegistered(write("ok.yaml", "url: x\nlogin: machine-id\npassword: y\n")) {
		t.Fatal("real login: want registered")
	}
}
