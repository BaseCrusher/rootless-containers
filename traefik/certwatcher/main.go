package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"
)

type pair struct {
	Cert string
	Key  string
}

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)

	certsDir := os.Getenv("CERTWATCHER_CERTS_DIR")
	if certsDir == "" {
		log.Fatal("CERTWATCHER_CERTS_DIR is required")
	}
	output := env("CERTWATCHER_OUTPUT", "/home/nonroot/config/dynamic/certs.yml")
	templatePath := env("CERTWATCHER_TEMPLATE", "/home/nonroot/certs.tmpl")

	interval, err := time.ParseDuration(env("CERTWATCHER_INTERVAL", "5s"))
	if err != nil {
		log.Fatalf("CERTWATCHER_INTERVAL: %v", err)
	}

	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		log.Fatalf("template: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		log.Fatalf("output directory: %v", err)
	}

	log.Printf("watching %s every %s, writing %s", certsDir, interval, output)

	var applied string
	for {
		pairs, fingerprint, err := scan(certsDir)
		switch {
		case err != nil:
			log.Printf("scan %s: %v", certsDir, err)
		case fingerprint != applied:
			if err := write(output, tmpl, pairs); err != nil {
				log.Printf("write %s: %v", output, err)
				break
			}
			applied = fingerprint
			log.Printf("wrote %s with %d certificate(s)", output, len(pairs))
		}
		time.Sleep(interval)
	}
}

func scan(dir string) ([]pair, string, error) {
	var pairs []pair

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".pem") {
			return nil
		}
		key := strings.TrimSuffix(path, ".pem") + ".key"
		if _, err := os.Stat(key); err != nil {
			return nil
		}
		pairs = append(pairs, pair{Cert: path, Key: key})
		return nil
	})
	if err != nil {
		return nil, "", err
	}

	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Cert < pairs[j].Cert })

	sum := sha256.New()
	for _, p := range pairs {
		content, err := os.ReadFile(p.Cert)
		if err != nil {
			return nil, "", err
		}
		sum.Write([]byte(p.Cert))
		sum.Write(content)
	}

	return pairs, hex.EncodeToString(sum.Sum(nil)), nil
}

func write(output string, tmpl *template.Template, pairs []pair) error {
	tmp, err := os.CreateTemp(filepath.Dir(output), ".certwatcher-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if err := tmpl.Execute(tmp, pairs); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}

	return os.Rename(tmp.Name(), output)
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
