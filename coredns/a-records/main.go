package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	prefix     = "COREDNSARECORDS_"
	defaultDir = "/container-supervisor/supervisor_environment"
)

var recordKey = regexp.MustCompile(`^COREDNS_([^_]+)__records___AT__(\d+)$`)

func expand(environ []string) map[string]string {
	maxIndex := map[string]int{}
	zones := map[string]string{}
	for _, kv := range environ {
		k, v, _ := strings.Cut(kv, "=")
		if m := recordKey.FindStringSubmatch(k); m != nil {
			if n, _ := strconv.Atoi(m[2]); n > maxIndex[m[1]] {
				maxIndex[m[1]] = n
			}
			continue
		}
		if g, ok := strings.CutPrefix(k, prefix); ok {
			zones[g] = v
		}
	}
	out := map[string]string{}
	for g, list := range zones {
		i := maxIndex[g]
		for _, ip := range strings.Split(list, ",") {
			ip = strings.TrimSpace(ip)
			if ip == "" {
				continue
			}
			i++
			out[fmt.Sprintf("COREDNS_%s__records___AT__%d", g, i)] = "IN A " + ip
		}
	}
	return out
}

func main() {
	dir := defaultDir
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	out := expand(os.Environ())
	if len(out) == 0 {
		return
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for name, val := range out {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(val), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
