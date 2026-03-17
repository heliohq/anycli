package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shipbase/anycli/internal/config"
	"github.com/shipbase/anycli/internal/registry"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth <tool>",
	Short: "Configure authentication for a tool",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		def, err := registry.Load(name)
		if err != nil {
			return err
		}

		if def.Auth == nil {
			fmt.Printf("%s does not require authentication\n", name)
			return nil
		}

		// Check if token is provided via flag
		token, _ := cmd.Flags().GetString("token")
		if token == "" {
			// Interactive prompt
			prompt := def.Auth.Prompt
			if prompt == "" {
				prompt = fmt.Sprintf("Enter %s", def.Auth.EnvVar)
			}
			fmt.Printf("%s: ", prompt)
			reader := bufio.NewReader(os.Stdin)
			token, _ = reader.ReadString('\n')
			token = strings.TrimSpace(token)
		}

		if token == "" {
			return fmt.Errorf("no credential provided")
		}

		// Save credential
		if err := config.EnsureDirs(); err != nil {
			return err
		}

		creds := map[string]string{
			def.Auth.EnvVar: token,
		}
		data, err := json.MarshalIndent(creds, "", "  ")
		if err != nil {
			return err
		}

		credPath := filepath.Join(config.CredentialsDir(), name+".json")
		if err := os.WriteFile(credPath, data, 0600); err != nil {
			return err
		}

		fmt.Printf("credentials saved for %s\n", name)
		return nil
	},
}

func init() {
	authCmd.Flags().String("token", "", "provide token non-interactively")
	rootCmd.AddCommand(authCmd)
}
