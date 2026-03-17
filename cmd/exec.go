package cmd

import (
	"fmt"
	"os"

	"github.com/shipbase/anycli/internal/exec"
	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:   "exec <tool> [args...]",
	Short: "Execute a tool through the middleware pipeline",
	Args:  cobra.MinimumNArgs(1),
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		toolArgs := args[1:]

		exitCode, err := exec.Run(name, toolArgs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "anycli: %s\n", err)
		}
		os.Exit(exitCode)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(execCmd)
}
