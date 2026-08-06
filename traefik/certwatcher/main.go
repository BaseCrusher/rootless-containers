package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
)

const marker = "# certwatcher "

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

	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		log.Fatalf("template: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		log.Fatalf("output directory: %v", err)
	}

	pairs, fingerprint, err := scan(certsDir)
	if err != nil {
		log.Printf("scan %s: %v", certsDir, err)
		return
	}
	if fingerprint == applied(output) {
		return
	}
	if err := write(output, tmpl, pairs, fingerprint); err != nil {
		log.Printf("write %s: %v", output, err)
		return
	}
	log.Printf("wrote %s with %d certificate(s)", output, len(pairs))
}

// applied returns the fingerprint of the certificates the output was last written
// from. The output is its own state: certwatcher stamps the fingerprint into the
// first line, so a run that changes nothing rewrites nothing.
func applied(output string) string {
	content, err := os.ReadFile(output)
	if err != nil {
		return ""
	}
	first, _, _ := strings.Cut(string(content), "\n")
	if !strings.HasPrefix(first, marker) {
		return ""
	}
	return strings.TrimPrefix(first, marker)
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

func write(output string, tmpl *template.Template, pairs []pair, fingerprint string) error {
	tmp, err := os.CreateTemp(filepath.Dir(output), ".certwatcher-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := fmt.Fprintf(tmp, "%s%s\n", marker, fingerprint); err != nil {
		tmp.Close()
		return err
	}
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
