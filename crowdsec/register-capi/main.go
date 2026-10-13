package main

import (
	"log"
	"os"
	"strings"
	"syscall"
)

const cscli = "/usr/local/bin/cscli"

func main() {
	log.SetFlags(0)

	if len(os.Args) < 2 || strings.HasPrefix(os.Args[1], "-") {
		log.Fatal("credentials file path is required as the first argument")
	}
	path := os.Args[1]

	if isRegistered(path) {
		log.Printf("CAPI already registered in %s, skipping", path)
		return
	}

	argv := []string{cscli, "capi", "register", "-f", path}
	if err := syscall.Exec(cscli, argv, os.Environ()); err != nil {
		log.Fatalf("exec %s: %v", cscli, err)
	}
}

// isRegistered reports whether path already holds CAPI credentials. Upstream
// ships this file empty (just url, or nothing), so a plain existence check would
// always skip; registration is done exactly when there is no non-empty login.
func isRegistered(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "login:") && strings.TrimSpace(line[len("login:"):]) != "" {
			return true
		}
	}
	return false
}
