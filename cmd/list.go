package cmd

import (
	"fmt"

	"github.com/shipbase/anycli/definitions"
	"github.com/shipbase/anycli/internal/registry"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available and installed wrappers",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Build set of installed tools
		installed, err := registry.List()
		if err != nil {
			return err
		}
		installedSet := make(map[string]bool)
		for _, name := range installed {
			installedSet[name] = true
		}

		// Get all bundled definitions
		bundled, err := definitions.List()
		if err != nil {
			return err
		}

		// Merge: bundled + installed (some may overlap)
		seen := make(map[string]bool)
		var all []struct {
			name        string
			description string
			status      string
		}

		for _, name := range bundled {
			seen[name] = true
			desc := ""
			status := "available"
			if installedSet[name] {
				status = "installed"
			}
			if def, err := definitions.LoadBundled(name); err == nil {
				desc = def.Description
			}
			all = append(all, struct {
				name        string
				description string
				status      string
			}{name, desc, status})
		}

		// Add installed-only (from --from, not bundled)
		for _, name := range installed {
			if seen[name] {
				continue
			}
			desc := ""
			if def, err := registry.Load(name); err == nil {
				desc = def.Description
			}
			all = append(all, struct {
				name        string
				description string
				status      string
			}{name, desc, "installed"})
		}

		if len(all) == 0 {
			fmt.Println("no wrappers available")
			return nil
		}

		for _, t := range all {
			fmt.Printf("%-20s %-12s %s\n", t.name, "["+t.status+"]", t.description)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
