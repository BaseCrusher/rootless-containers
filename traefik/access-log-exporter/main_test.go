package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fixture struct {
	t      *testing.T
	e      *exporter
	bodies []string
	reject func(body string) int
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	f := &fixture{t: t}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		f.bodies = append(f.bodies, string(body))
		if f.reject != nil {
			w.WriteHeader(f.reject(string(body)))
		}
	}))
	t.Cleanup(server.Close)

	logPath := filepath.Join(t.TempDir(), "access.log")
	f.e = &exporter{
		file:        logPath,
		state:       logPath + ".offset",
		batch:       1000,
		maxSize:     -1,
		url:         server.URL,
		contentType: "application/x-ndjson",
		headers:     http.Header{},
		client:      server.Client(),
	}
	return f
}

func (f *fixture) write(content string) {
	f.t.Helper()
	file, err := os.OpenFile(f.e.file, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err := file.WriteString(content); err != nil {
		f.t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) tick() {
	f.t.Helper()
	if err := f.e.run(); err != nil {
		f.t.Fatalf("tick: %v", err)
	}
}

func (f *fixture) tickFails() {
	f.t.Helper()
	if err := f.e.run(); err == nil {
		f.t.Fatal("tick succeeded, want failure")
	}
}

func (f *fixture) sent() []string {
	bodies := f.bodies
	f.bodies = nil
	return bodies
}

func (f *fixture) size() int64 {
	f.t.Helper()
	info, err := os.Stat(f.e.file)
	if err != nil {
		f.t.Fatal(err)
	}
	return info.Size()
}

func (f *fixture) offsets() (shipped, echoed int64) {
	f.t.Helper()
	return loadState(f.e.state)
}

// captureStdout redirects os.Stdout for the rest of the test and returns everything
// written to it so far on each call.
func captureStdout(t *testing.T) func() string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "stdout")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = file
	t.Cleanup(func() {
		os.Stdout = original
		file.Close()
	})

	return func() string {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(content)
	}
}

func TestMissingFileAndEmptyTickSendNothing(t *testing.T) {
	f := newFixture(t)

	f.tick()
	f.write("")
	f.tick()

	if got := f.sent(); len(got) != 0 {
		t.Errorf("sent %v, want no requests", got)
	}
	if _, err := os.Stat(f.e.state); err == nil {
		t.Error("state written for a missing log file")
	}
}

func TestOffsetAdvancesAcrossTicks(t *testing.T) {
	f := newFixture(t)

	f.write("{\"a\":1}\n{\"a\":2}\n")
	f.tick()
	if got := f.sent(); len(got) != 1 || got[0] != "{\"a\":1}\n{\"a\":2}\n" {
		t.Fatalf("first tick sent %q", got)
	}
	if shipped, _ := f.offsets(); shipped != f.size() {
		t.Errorf("shipped = %d, want %d", shipped, f.size())
	}

	f.tick()
	if got := f.sent(); len(got) != 0 {
		t.Errorf("second tick sent %q, want nothing new", got)
	}

	f.write("{\"a\":3}\n")
	f.tick()
	if got := f.sent(); len(got) != 1 || got[0] != "{\"a\":3}\n" {
		t.Errorf("third tick sent %q, want only the new line", got)
	}
}

func TestPartialLineHeldBackUntilComplete(t *testing.T) {
	f := newFixture(t)

	f.write("{\"a\":1}\n{\"a\":2")
	f.tick()
	if got := f.sent(); len(got) != 1 || got[0] != "{\"a\":1}\n" {
		t.Fatalf("sent %q, want only the complete line", got)
	}
	if shipped, _ := f.offsets(); shipped != 8 {
		t.Fatalf("shipped = %d, want 8 (before the partial line)", shipped)
	}

	f.write("}\n")
	f.tick()
	if got := f.sent(); len(got) != 1 || got[0] != "{\"a\":2}\n" {
		t.Errorf("sent %q, want the completed line once", got)
	}
	if shipped, _ := f.offsets(); shipped != f.size() {
		t.Errorf("shipped = %d, want %d", shipped, f.size())
	}
}

func TestResetsAfterExternalTruncate(t *testing.T) {
	f := newFixture(t)

	f.write("{\"a\":1}\n{\"a\":2}\n")
	f.tick()
	f.sent()

	if err := os.Truncate(f.e.file, 0); err != nil {
		t.Fatal(err)
	}
	f.write("{\"b\":1}\n")
	f.tick()

	if got := f.sent(); len(got) != 1 || got[0] != "{\"b\":1}\n" {
		t.Errorf("sent %q, want the new file read from 0", got)
	}
}

func TestTransientFailureKeepsOffsetDoesNotReEchoAndDoesNotRotate(t *testing.T) {
	f := newFixture(t)
	f.e.echo = true
	f.e.maxSize = 0
	stdout := captureStdout(t)

	f.reject = func(string) int { return http.StatusInternalServerError }
	f.write("{\"a\":1}\n")
	f.tickFails()

	if shipped, echoed := f.offsets(); shipped != 0 || echoed != 8 {
		t.Errorf("offsets = %d/%d, want shipped 0 and echoed 8", shipped, echoed)
	}
	if f.size() == 0 {
		t.Error("rotated despite an unshipped line")
	}

	f.reject = nil
	f.tick()

	if got := f.sent(); len(got) != 2 || got[1] != "{\"a\":1}\n" {
		t.Errorf("sent %q, want the line retried", got)
	}
	if got := stdout(); got != "{\"a\":1}\n" {
		t.Errorf("stdout = %q, want the line echoed exactly once", got)
	}
}

func TestBadRequestSkipsOnlyTheBadLine(t *testing.T) {
	f := newFixture(t)

	f.reject = func(body string) int {
		if strings.Contains(body, "oops") {
			return http.StatusBadRequest
		}
		return http.StatusOK
	}
	f.write("{\"a\":1}\noops\n{\"a\":2}\n")
	f.tick()

	want := []string{"{\"a\":1}\noops\n{\"a\":2}\n", "{\"a\":1}\n", "oops\n", "{\"a\":2}\n"}
	got := f.sent()
	if len(got) != len(want) {
		t.Fatalf("sent %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("request %d = %q, want %q", i, got[i], want[i])
		}
	}
	if shipped, _ := f.offsets(); shipped != f.size() {
		t.Errorf("shipped = %d, want %d (past the bad line)", shipped, f.size())
	}

	f.tick()
	if got := f.sent(); len(got) != 0 {
		t.Errorf("bad line resent as %q, want nothing", got)
	}
}

func TestRotation(t *testing.T) {
	f := newFixture(t)

	f.write("{\"a\":1}\n")
	f.tick()
	if f.size() != 8 {
		t.Errorf("MAX_SIZE=-1 rotated, size = %d", f.size())
	}

	f.e.maxSize = 0
	f.tick()
	if f.size() != 0 {
		t.Errorf("MAX_SIZE=0 did not rotate, size = %d", f.size())
	}
	if shipped, echoed := f.offsets(); shipped != 0 || echoed != 0 {
		t.Errorf("offsets = %d/%d after rotation, want 0/0", shipped, echoed)
	}

	f.sent()
	f.write("{\"a\":2}\n")
	f.tick()
	if got := f.sent(); len(got) != 1 || got[0] != "{\"a\":2}\n" {
		t.Errorf("sent %q after rotation, want the new line from 0", got)
	}
	if f.size() != 0 {
		t.Errorf("MAX_SIZE=0 did not rotate the second time, size = %d", f.size())
	}

	f.e.maxSize = 16
	f.write("{\"a\":3}\n")
	f.tick()
	if f.size() != 8 {
		t.Errorf("rotated below MAX_SIZE, size = %d", f.size())
	}
}
