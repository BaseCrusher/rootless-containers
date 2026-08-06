package main

import (
	"os"
	"path/filepath"
	"testing"
	"text/template"
)

func TestScan(t *testing.T) {
	dir := t.TempDir()
	put := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	put("example.com.pem", "cert-a")
	put("example.com.key", "key-a")
	put("example.com.json", "{}")
	put("orphan.com.pem", "cert-orphan")

	pairs, fingerprint, err := scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 1 {
		t.Fatalf("got %d pairs, want 1: %v", len(pairs), pairs)
	}
	if got := filepath.Base(pairs[0].Cert); got != "example.com.pem" {
		t.Errorf("cert = %q", got)
	}
	if got := filepath.Base(pairs[0].Key); got != "example.com.key" {
		t.Errorf("key = %q", got)
	}

	if _, unchanged, err := scan(dir); err != nil || unchanged != fingerprint {
		t.Errorf("fingerprint changed without a cert change: %v %v", unchanged, err)
	}

	put("example.com.pem", "cert-a-renewed")
	_, renewed, err := scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if renewed == fingerprint {
		t.Error("fingerprint unchanged after renewal")
	}

	put("other.com.pem", "cert-b")
	put("other.com.key", "key-b")
	pairs, added, err := scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if added == renewed {
		t.Error("fingerprint unchanged after a new domain appeared")
	}
	if len(pairs) != 2 {
		t.Fatalf("got %d pairs, want 2: %v", len(pairs), pairs)
	}
	if pairs[0].Cert > pairs[1].Cert {
		t.Errorf("pairs not sorted: %v", pairs)
	}
}

func TestWriteRendersTemplate(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "certs.yml")

	tmpl, err := template.ParseFiles("../certs.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	if err := write(output, tmpl, []pair{{Cert: "/certs/example.com.pem", Key: "/certs/example.com.key"}}, "abc123"); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	want := "# certwatcher abc123\ntls:\n  certificates:\n    - certFile: /certs/example.com.pem\n      keyFile: /certs/example.com.key\n"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}

	leftovers, err := filepath.Glob(filepath.Join(dir, ".certwatcher-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(leftovers) != 0 {
		t.Errorf("temp files left behind: %v", leftovers)
	}
}

// The output carries the fingerprint it was written from, so a run that finds the
// same certificates rewrites nothing — the state a long-running watcher used to
// keep in memory now survives across the runs of a ticker.
func TestAppliedRoundTripsThroughTheOutput(t *testing.T) {
	certs := t.TempDir()
	output := filepath.Join(t.TempDir(), "certs.yml")

	put := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(certs, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	put("example.com.pem", "cert-a")
	put("example.com.key", "key-a")

	tmpl, err := template.ParseFiles("../certs.tmpl")
	if err != nil {
		t.Fatal(err)
	}

	if got := applied(output); got != "" {
		t.Errorf("applied on a missing output = %q, want empty", got)
	}

	pairs, fingerprint, err := scan(certs)
	if err != nil {
		t.Fatal(err)
	}
	if err := write(output, tmpl, pairs, fingerprint); err != nil {
		t.Fatal(err)
	}
	if got := applied(output); got != fingerprint {
		t.Fatalf("applied = %q, want %q", got, fingerprint)
	}

	put("example.com.pem", "cert-a-renewed")
	if _, renewed, err := scan(certs); err != nil || renewed == applied(output) {
		t.Errorf("renewal not detected: %v %v", renewed, err)
	}

	if err := os.WriteFile(output, []byte("tls: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := applied(output); got != "" {
		t.Errorf("applied on an output without a marker = %q, want empty", got)
	}
}
