package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/shipbase/anycli/definitions"
	"github.com/shipbase/anycli/internal/config"
	"github.com/shipbase/anycli/internal/installer"
	"github.com/shipbase/anycli/internal/registry"
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install <tool>",
	Short: "Install a CLI wrapper",
	Long:  "Download the tool binary, install the wrapper definition, and create a PATH shim.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		mode, _ := cmd.Flags().GetString("conflict-policy")

		// Check if already installed in anycli
		if _, err := registry.Load(name); err == nil {
			fmt.Printf("%s is already installed\n", name)
			return nil
		}

		// Load definition: --from file > bundled
		var def *registry.Definition
		source, _ := cmd.Flags().GetString("from")
		if source != "" {
			d, err := loadFromFile(source)
			if err != nil {
				return err
			}
			def = d
		} else {
			d, err := definitions.LoadBundled(name)
			if err != nil {
				return fmt.Errorf("unknown tool %q (not bundled); use --from <path> to install from a local definition", name)
			}
			def = d
		}

		// Check if the tool already exists in PATH
		if mode == "" {
			existingPath, err := findExistingBinary(name)
			if err == nil && existingPath != "" {
				fmt.Fprintf(os.Stderr, "conflict: %s already exists at %s\n", name, existingPath)
				fmt.Fprintf(os.Stderr, "\nrerun with --conflict-policy:\n")
				fmt.Fprintf(os.Stderr, "  --conflict-policy override   download new binary, replace existing\n")
				fmt.Fprintf(os.Stderr, "  --conflict-policy link       wrap existing binary with anycli middleware\n")
				fmt.Fprintf(os.Stderr, "\nif you are an agent, ask the user which policy to use.\n")
				return fmt.Errorf("installation aborted due to conflict")
			}
		}

		if mode == "link" {
			// Link mode: wrap existing binary, skip download
			existingPath, err := findExistingBinary(name)
			if err != nil {
				return fmt.Errorf("cannot find existing %s: %w", name, err)
			}
			def.Resolve = existingPath
			def.Source = nil // don't download
			fmt.Printf("linking to existing %s at %s\n", name, existingPath)
		}

		// Download the real binary if source is configured (override mode or no conflict)
		if def.Source != nil {
			result, err := installer.Install(def)
			if err != nil {
				return fmt.Errorf("failed to install %s: %w", name, err)
			}
			def.Resolve = result.BinaryPath
			fmt.Printf("downloaded %s v%s\n", name, result.Version)
		}

		// Save definition to registry
		if err := registry.Save(def); err != nil {
			return err
		}

		// Create shim symlink
		if err := createShim(name); err != nil {
			return err
		}

		fmt.Printf("installed %s\n", name)
		return nil
	},
}

// findExistingBinary searches PATH for an existing binary, skipping the anycli shim dir.
func findExistingBinary(name string) (string, error) {
	shimDir := config.BinDir()
	pathEnv := os.Getenv("PATH")
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == shimDir {
			continue
		}
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", exec.ErrNotFound
}

func loadFromFile(path string) (*registry.Definition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read definition file: %w", err)
	}

	var def registry.Definition
	if err := registry.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("invalid definition file: %w", err)
	}
	return &def, nil
}

// createShim creates a symlink in the bin dir pointing to the anycli binary.
func createShim(name string) error {
	if err := config.EnsureDirs(); err != nil {
		return err
	}

	anycliBin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine anycli binary path: %w", err)
	}
	anycliBin, err = filepath.EvalSymlinks(anycliBin)
	if err != nil {
		return err
	}

	shimPath := filepath.Join(config.BinDir(), name)
	os.Remove(shimPath)

	if err := os.Symlink(anycliBin, shimPath); err != nil {
		return fmt.Errorf("cannot create shim: %w", err)
	}
	return nil
}

func init() {
	installCmd.Flags().String("from", "", "install from a local JSON definition file")
	installCmd.Flags().String("conflict-policy", "", "conflict resolution when tool exists in PATH: override or link")
	rootCmd.AddCommand(installCmd)
}
