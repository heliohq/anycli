package main

import (
	"os"
	"path/filepath"

	"github.com/shipbase/anycli/cmd"
	"github.com/shipbase/anycli/internal/shim"
)

func main() {
	// Busybox-style detection: if invoked via a symlink name other than "anycli",
	// treat it as a shim call and delegate to exec.
	name := filepath.Base(os.Args[0])
	if name != "anycli" {
		shim.Run(name, os.Args[1:])
		return
	}

	cmd.Execute()
}
