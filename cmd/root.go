package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "anycli",
	Short: "Make every tool agent-native",
	Long:  "AnyCLI wraps existing CLIs into agent-friendly interfaces via PATH shims with a declarative JSON-based middleware pipeline.",
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
