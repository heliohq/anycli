package cmd

import (
	"fmt"

	"github.com/shipbase/anycli/internal/registry"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed wrappers",
	RunE: func(cmd *cobra.Command, args []string) error {
		names, err := registry.List()
		if err != nil {
			return err
		}
		if len(names) == 0 {
			fmt.Println("no wrappers installed")
			return nil
		}
		for _, name := range names {
			def, err := registry.Load(name)
			if err != nil {
				fmt.Printf("%-20s (error loading definition)\n", name)
				continue
			}
			fmt.Printf("%-20s %s\n", name, def.Description)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
