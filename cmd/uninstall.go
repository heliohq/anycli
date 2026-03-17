package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/shipbase/anycli/internal/config"
	"github.com/shipbase/anycli/internal/registry"
	"github.com/spf13/cobra"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall <tool>",
	Short: "Uninstall a CLI wrapper",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		// Remove shim
		shimPath := filepath.Join(config.BinDir(), name)
		os.Remove(shimPath)

		// Remove definition
		if err := registry.Remove(name); err != nil {
			return fmt.Errorf("cannot remove wrapper: %w", err)
		}

		// Remove credentials if present
		credPath := filepath.Join(config.CredentialsDir(), name+".json")
		os.Remove(credPath)

		fmt.Printf("uninstalled %s\n", name)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(uninstallCmd)
}
