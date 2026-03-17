package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

var rootCmd = &cobra.Command{
	Use:   "any",
	Short: "Wrap authenticated cloud service CLIs/APIs for agents",
	Long:  "AnyCLI wraps authenticated cloud service CLIs/APIs (gh, wrangler, aws, gcloud, etc.) into agent-friendly interfaces with automatic credential injection and middleware hooks.",
}

// SetVersion sets the version string.
func SetVersion(v string) {
	version = v
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
