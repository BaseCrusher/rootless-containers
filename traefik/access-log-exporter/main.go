package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type exporter struct {
	file    string
	state   string
	batch   int
	maxSize int64
	echo    bool

	url         string
	contentType string
	headers     http.Header
	client      *http.Client
}

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	log.SetPrefix("access-log-exporter: ")

	url := env("ACCESSLOGEXPORTER_URL", "")
	if url == "" {
		log.Fatal("ACCESSLOGEXPORTER_URL is required")
	}
	file := env("ACCESSLOGEXPORTER_FILE", "/home/nonroot/config/access.log")

	batch, err := strconv.Atoi(env("ACCESSLOGEXPORTER_BATCH", "1000"))
	if err != nil || batch < 1 {
		log.Fatalf("ACCESSLOGEXPORTER_BATCH: want a positive line count, got %q", os.Getenv("ACCESSLOGEXPORTER_BATCH"))
	}
	maxSize, err := strconv.ParseInt(env("ACCESSLOGEXPORTER_MAX_SIZE", "67108864"), 10, 64)
	if err != nil || maxSize < -1 {
		log.Fatalf("ACCESSLOGEXPORTER_MAX_SIZE: want a size in bytes, 0 or -1, got %q", os.Getenv("ACCESSLOGEXPORTER_MAX_SIZE"))
	}

	e := &exporter{
		file:        file,
		state:       env("ACCESSLOGEXPORTER_STATE", file+".offset"),
		batch:       batch,
		maxSize:     maxSize,
		echo:        env("ACCESSLOGEXPORTER_ECHO", "") == "true",
		url:         url,
		contentType: env("ACCESSLOGEXPORTER_CONTENT_TYPE", "application/x-ndjson"),
		headers:     authHeader(),
		client:      &http.Client{Timeout: 30 * time.Second},
	}
	if err := e.run(); err != nil {
		log.Fatal(err)
	}
}

func (e *exporter) run() error {
	shipped, echoed := loadState(e.state)
	savedShipped, savedEchoed := shipped, echoed

	f, err := os.Open(e.file)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}
	size := info.Size()
	if size < shipped {
		shipped = 0
	}
	if size < echoed {
		echoed = shipped
	}

	var failure error
	for shipped < size {
		lines, end, err := read(f, shipped, e.batch)
		if err != nil {
			failure = err
			break
		}
		if len(lines) == 0 {
			break
		}
		if e.echo {
			echoed = echoLines(lines, shipped, echoed)
		}
		shipped, err = e.ship(lines, shipped, end)
		if err != nil {
			failure = err
			break
		}
	}

	if failure == nil && shipped == size && e.maxSize >= 0 && size > e.maxSize {
		// ponytail: Traefik opens the access log O_APPEND, so truncating is the whole
		// rotation — the next write lands at 0 and leaves no sparse hole. No SIGUSR1,
		// no /proc scan for its PID. Lines written between the Stat above and here are
		// lost; CrowdSec decides over many requests, so it is not worth a rename dance.
		if err := os.Truncate(e.file, 0); err != nil {
			failure = err
		} else {
			shipped, echoed = 0, 0
			log.Printf("rotated %s at %d bytes", e.file, size)
		}
	}

	if shipped != savedShipped || echoed != savedEchoed {
		if err := saveState(e.state, shipped, echoed); err != nil {
			return err
		}
	}
	return failure
}

// read returns up to limit whole lines starting at offset. A trailing line with no
// newline yet is left for the next tick.
func read(f *os.File, offset int64, limit int) ([][]byte, int64, error) {
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, 0, err
	}

	r := bufio.NewReader(f)
	var lines [][]byte
	end := offset
	for len(lines) < limit {
		line, err := r.ReadBytes('\n')
		if err != nil {
			break
		}
		lines = append(lines, line)
		end += int64(len(line))
	}
	return lines, end, nil
}

func echoLines(lines [][]byte, offset, echoed int64) int64 {
	for _, line := range lines {
		offset += int64(len(line))
		if offset > echoed {
			os.Stdout.Write(line)
			echoed = offset
		}
	}
	return echoed
}

// ship POSTs the batch and returns the offset reached. The offset only advances past
// bytes the far end will never accept (400) or has accepted (200).
func (e *exporter) ship(lines [][]byte, offset, end int64) (int64, error) {
	status, err := e.post(bytes.Join(lines, nil))
	switch {
	case err != nil:
		return offset, fmt.Errorf("post %d line(s): %w", len(lines), err)
	case status == http.StatusOK:
		return end, nil
	case status != http.StatusBadRequest:
		return offset, fmt.Errorf("post %d line(s): %d %s", len(lines), status, http.StatusText(status))
	}

	cursor := offset
	for _, line := range lines {
		status, err := e.post(line)
		switch {
		case err != nil:
			return cursor, fmt.Errorf("post 1 line: %w", err)
		case status == http.StatusOK:
		case status == http.StatusBadRequest:
			log.Printf("skipping line at offset %d: rejected as malformed JSON, is TRAEFIK_ACCESSLOG_FORMAT=json set?", cursor)
		default:
			return cursor, fmt.Errorf("post 1 line: %d %s", status, http.StatusText(status))
		}
		cursor += int64(len(line))
	}
	return cursor, nil
}

func (e *exporter) post(body []byte) (int, error) {
	req, err := http.NewRequest(http.MethodPost, e.url, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header = e.headers.Clone()
	req.Header.Set("Content-Type", e.contentType)

	resp, err := e.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	return resp.StatusCode, nil
}

func loadState(path string) (shipped, echoed int64) {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0, 0
	}
	fmt.Sscan(string(content), &shipped, &echoed)
	if shipped < 0 || echoed < 0 {
		return 0, 0
	}
	return shipped, echoed
}

func saveState(path string, shipped, echoed int64) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".access-log-exporter-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := fmt.Fprintf(tmp, "%d %d\n", shipped, echoed); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func authHeader() http.Header {
	headers := http.Header{}
	if key := env("ACCESSLOGEXPORTER_HEADER_KEY", ""); key != "" {
		headers.Set(key, env("ACCESSLOGEXPORTER_HEADER_VALUE", ""))
	}
	return headers
}

// env returns the value of key, or the contents of the file named by key+"_FILE"
// when that is set (Docker/Kubernetes secrets), or fallback.
func env(key, fallback string) string {
	if path := os.Getenv(key + "_FILE"); path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			log.Fatalf("%s_FILE: %v", key, err)
		}
		return strings.TrimSpace(string(b))
	}
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
