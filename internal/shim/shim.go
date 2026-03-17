package shim

import (
	"fmt"
	"os"

	"github.com/shipbase/anycli/internal/exec"
)

// Run is called when anycli is invoked via a symlink (busybox-style).
// It delegates to exec.Run with the symlink name as the tool.
func Run(name string, args []string) {
	exitCode, err := exec.Run(name, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "anycli: %s: %s\n", name, err)
	}
	os.Exit(exitCode)
}
