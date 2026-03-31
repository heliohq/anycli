package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shipbase/anycli/internal/config"
	"github.com/shipbase/anycli/internal/credential"
	"github.com/shipbase/anycli/internal/registry"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth <tool>",
	Short: "Configure authentication for a tool",
	Args:  cobra.ExactArgs(1),
	RunE:  runAuth,
}

func init() {
	authCmd.Flags().StringSlice("set", nil, "set a credential value (key=value, can be repeated)")
	authCmd.Flags().Bool("json", false, "output in JSON format")
	rootCmd.AddCommand(authCmd)
}

func runAuth(cmd *cobra.Command, args []string) error {
	name := args[0]
	jsonOutput, _ := cmd.Flags().GetBool("json")
	setValues, _ := cmd.Flags().GetStringSlice("set")

	def, err := registry.Load(name)
	if err != nil {
		return authError(jsonOutput, name, err.Error())
	}

	// No auth required
	if def.Auth == nil {
		if jsonOutput {
			return writeJSON(map[string]interface{}{
				"ok":      true,
				"tool":    name,
				"message": fmt.Sprintf("%s does not require authentication", name),
			})
		}
		fmt.Printf("%s does not require authentication\n", name)
		return nil
	}

	// Vault mode: reject local credential writes
	vaultCfg, vaultErr := credential.GetVaultConfig()
	if vaultErr != nil {
		// Partial vault configuration — this is an error
		return authError(jsonOutput, name, fmt.Sprintf("vault configuration error: %v", vaultErr))
	}
	if vaultCfg != nil {
		msg := fmt.Sprintf("credentials for %q are managed by vault service", name)
		if jsonOutput {
			writeJSONError(name, msg)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Error: %s.\nConfigure credentials via the platform dashboard.\n", msg)
		os.Exit(1)
	}

	// Build valid auth flag map: auth_flag -> local_key
	flagToKey := make(map[string]string)
	for _, cb := range def.Auth.Credentials {
		authFlag := cb.Source.AuthFlag
		if authFlag == "" {
			authFlag = deriveAuthFlag(cb.Source.LocalKey)
		}
		flagToKey[authFlag] = cb.Source.LocalKey
	}

	// No --set provided: show usage error with valid keys
	if len(setValues) == 0 {
		var validFlags []string
		for flag := range flagToKey {
			validFlags = append(validFlags, flag)
		}
		msg := fmt.Sprintf("no credentials provided; use --set with one of: %s", strings.Join(validFlags, ", "))
		return authError(jsonOutput, name, msg)
	}

	// Load existing credentials to merge with
	credMap := make(map[string]string)
	existingPath := filepath.Join(config.CredentialsDir(), name+".json")
	if existingData, err := os.ReadFile(existingPath); err == nil {
		_ = json.Unmarshal(existingData, &credMap) // ignore errors, start fresh if invalid
	}

	// Apply --set values (overwriting existing keys)
	for _, kv := range setValues {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return authError(jsonOutput, name, fmt.Sprintf("invalid --set format %q; expected key=value", kv))
		}
		key := parts[0]
		value := parts[1]

		localKey, ok := flagToKey[key]
		if !ok {
			var validFlags []string
			for flag := range flagToKey {
				validFlags = append(validFlags, flag)
			}
			return authError(jsonOutput, name, fmt.Sprintf("unknown auth key %q; valid keys: %s", key, strings.Join(validFlags, ", ")))
		}
		credMap[localKey] = value
	}

	// Write credentials file
	if err := config.EnsureDirs(); err != nil {
		return authError(jsonOutput, name, fmt.Sprintf("failed to create directories: %v", err))
	}

	data, err := json.MarshalIndent(credMap, "", "  ")
	if err != nil {
		return authError(jsonOutput, name, fmt.Sprintf("failed to marshal credentials: %v", err))
	}

	credPath := filepath.Join(config.CredentialsDir(), name+".json")
	if err := os.WriteFile(credPath, data, 0600); err != nil {
		return authError(jsonOutput, name, fmt.Sprintf("failed to write credentials: %v", err))
	}

	// Success output
	if jsonOutput {
		var keys []string
		for k := range credMap {
			keys = append(keys, k)
		}
		return writeJSON(map[string]interface{}{
			"ok":   true,
			"tool": name,
			"keys": keys,
		})
	}

	fmt.Printf("credentials saved for %s\n", name)
	return nil
}

// deriveAuthFlag converts a local_key like "GH_TOKEN" to an auth flag like "gh-token".
func deriveAuthFlag(localKey string) string {
	return strings.ToLower(strings.ReplaceAll(localKey, "_", "-"))
}

// authError outputs an error in JSON or prose format.
// When JSON output is enabled, it writes JSON to stdout and exits with code 1.
// When prose output is used, it returns an error for cobra to handle.
func authError(jsonOutput bool, tool string, msg string) error {
	if jsonOutput {
		writeJSONError(tool, msg)
		os.Exit(1)
	}
	return fmt.Errorf("%s", msg)
}

// writeJSON writes a JSON object to stdout.
func writeJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// writeJSONError writes a JSON error object to stdout.
func writeJSONError(tool string, msg string) {
	data := map[string]interface{}{
		"ok":    false,
		"error": msg,
		"tool":  tool,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(data)
}
