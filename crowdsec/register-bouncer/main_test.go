package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveKey(t *testing.T) {
	if _, err := resolveKey("", "", ""); err == nil {
		t.Fatal("no source: want error")
	}
	if _, err := resolveKey("a", "", "SOME_ENV"); err == nil {
		t.Fatal("two sources: want error")
	}

	if got, _ := resolveKey("literal", "", ""); got != "literal" {
		t.Fatalf("raw: got %q", got)
	}

	t.Setenv("BOUNCER_KEY_TRAEFIK", "from-env")
	if got, _ := resolveKey("", "BOUNCER_KEY_TRAEFIK", ""); got != "from-env" {
		t.Fatalf("env: got %q", got)
	}
	if _, err := resolveKey("", "MISSING_KEY", ""); err == nil {
		t.Fatal("unset env: want error")
	}

	f := filepath.Join(t.TempDir(), "key")
	os.WriteFile(f, []byte("from-file\n"), 0o600)
	if got, _ := resolveKey("", "", f); got != "from-file" {
		t.Fatalf("file: got %q", got)
	}
}
