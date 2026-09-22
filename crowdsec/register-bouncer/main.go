package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"syscall"
)

const cscli = "/usr/local/bin/cscli"

func main() {
	log.SetFlags(0)

	if len(os.Args) < 2 || strings.HasPrefix(os.Args[1], "-") {
		log.Fatal("bouncer name is required as the first argument")
	}
	name := os.Args[1]

	fs := flag.NewFlagSet("register-bouncer", flag.ExitOnError)
	keyRaw := fs.String("key-raw", "", "bouncer key given literally")
	keyEnv := fs.String("key-env", "", "name of an env var holding the bouncer key")
	keyFile := fs.String("key-file", "", "path to a file holding the bouncer key")
	fs.Parse(os.Args[2:])

	key, err := resolveKey(*keyRaw, *keyEnv, *keyFile)
	if err != nil {
		log.Fatal(err)
	}

	argv := []string{cscli, "bouncers", "add", name, "-k", key}
	if err := syscall.Exec(cscli, argv, os.Environ()); err != nil {
		log.Fatalf("exec %s: %v", cscli, err)
	}
}

// resolveKey returns the key from exactly one of the three sources.
func resolveKey(raw, env, file string) (string, error) {
	set := 0
	for _, v := range []string{raw, env, file} {
		if v != "" {
			set++
		}
	}
	if set != 1 {
		return "", fmt.Errorf("exactly one of --key-raw, --key-env, --key-file is required")
	}
	switch {
	case raw != "":
		return raw, nil
	case env != "":
		v, ok := os.LookupEnv(env)
		if !ok {
			return "", fmt.Errorf("env var %s is not set", env)
		}
		return v, nil
	default:
		b, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("key-file: %w", err)
		}
		return strings.TrimSpace(string(b)), nil
	}
}
