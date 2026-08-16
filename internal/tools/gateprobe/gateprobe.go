// Package gateprobe is the built-in policy-gate E2E probe: a hidden,
// credential-free test service whose runnable leaves echo a local receipt and
// never make a network call. `probe send` (gate-probe.probe_send) is the
// policy gate's subject; `probe copy` (gate-probe.probe_copy) is the
// filesystem seam's, reading one path and writing another so a host can prove
// where its Config.FS actually put the bytes. It exists so an end-to-end suite can exercise a host's full
// inspect → decide → execute path without depending on a real provider. The
// leaf is annotated side_effect=true so the consumer's policy layer gates it
// exactly like a real mutating command. The definition (definitions/tools/gate-probe.json)
// declares no auth block: execution needs no credentials, the engine never
// calls the resolver, and RunE reads none.
package gateprobe

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// Service implements the built-in gate-probe tool. It satisfies tools.Service
// by duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
	// FS is where `probe copy` reads and writes; nil = the os package.
	FS execution.FileSystem
}

// Execute runs one gate-probe subcommand. env is ignored: the probe is
// credential-free by contract (no auth block in its definition).
func (s *Service) Execute(ctx context.Context, args []string, _ map[string]string) (execution.Result, error) {
	root := s.newRoot()
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(s.stderr(), err)
		return execution.Failure(err), nil
	}
	return execution.Result{}, nil
}

func (s *Service) newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "gate-probe",
		Short:         "Policy-gate E2E probe (test-only)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")
	root.AddCommand(s.newProbeCmd())
	return root
}

// newProbeCmd builds the hidden `probe` group with its single `send` leaf.
// The leaf is a local no-op echo: zero network, zero credentials.
func (s *Service) newProbeCmd() *cobra.Command {
	probe := &cobra.Command{
		Use:    "probe",
		Short:  "Policy-gate probe verbs",
		Hidden: true,
	}

	var note string
	send := &cobra.Command{
		Use:         "send",
		Short:       "Echo a local probe receipt (no-op, zero network)",
		Hidden:      true,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"}, // gated like a real mutating command
		RunE: func(cmd *cobra.Command, _ []string) error {
			receipt := fmt.Sprintf(`{"tool":"gate-probe","action":"gate-probe.probe_send","status":"sent","note":%q}`, note)
			fmt.Fprintln(cmd.OutOrStdout(), receipt)
			return nil
		},
	}
	send.Flags().StringVar(&note, "note", "", "opaque marker echoed back in the receipt (lets tests vary argv)")

	probe.AddCommand(send)
	probe.AddCommand(s.newCopyCmd())
	return probe
}

// newCopyCmd is the filesystem seam's subject: it reads --in and writes --out,
// and reports the byte count it moved. A host that relays file access somewhere
// else can run this and then look at the machine it expected the bytes to land
// on — which is the one thing a probe that only echoes cannot show.
func (s *Service) newCopyCmd() *cobra.Command {
	var in, out string
	copyCmd := &cobra.Command{
		Use:         "copy",
		Short:       "Copy one local file to another (no-op elsewhere, zero network)",
		Hidden:      true,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"}, // it writes a file
		RunE: func(cmd *cobra.Command, _ []string) error {
			if in == "" || out == "" {
				return fmt.Errorf("probe copy requires --in and --out")
			}
			info, err := s.FS.Stat(in)
			if err != nil {
				return err
			}
			data, err := s.FS.ReadFile(in)
			if err != nil {
				return err
			}
			if err := s.FS.WriteFile(out, data, 0o644); err != nil {
				return err
			}
			receipt := fmt.Sprintf(
				`{"tool":"gate-probe","action":"gate-probe.probe_copy","status":"copied","in":%q,"out":%q,"bytes":%d,"stat_bytes":%d}`,
				in, out, len(data), info.Size())
			fmt.Fprintln(cmd.OutOrStdout(), receipt)
			return nil
		},
	}
	copyCmd.Flags().StringVar(&in, "in", "", "path to read")
	copyCmd.Flags().StringVar(&out, "out", "", "path to write")
	return copyCmd
}

func (s *Service) stdout() io.Writer {
	if s.Out != nil {
		return s.Out
	}
	return os.Stdout
}

func (s *Service) stderr() io.Writer {
	if s.Err != nil {
		return s.Err
	}
	return os.Stderr
}

// NewCommandTree returns the full command tree for dry-run parsing and
// traversal (tools.Service seam). The returned commands are never
// executed by Inspect/lint/policy consumers.
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot() }
