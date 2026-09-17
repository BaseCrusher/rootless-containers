package main

import (
	"net/http"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		os.Exit(1)
	}
	resp, err := http.Get(os.Args[1])
	if err != nil || resp.StatusCode != http.StatusOK {
		os.Exit(1)
	}
	os.Exit(0)
}
