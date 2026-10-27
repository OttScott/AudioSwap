package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	executable, err := os.Executable()
	if err == nil {
		path := filepath.Join(filepath.Dir(executable), "audioswap.json")
		err = run(os.Args[1:], os.Stdout, path, openAudio)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "audioswap: %v\n", err)
		os.Exit(1)
	}
}
