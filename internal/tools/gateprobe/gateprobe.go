// Package gateprobe is the built-in approval-gate E2E probe (design 318 §E2E
// Testing Harness): a hidden, credential-free test service whose single
// runnable leaf `probe send` (action gate-probe.probe_send) echoes a local
// receipt and never makes a network call. It exists so the approval-gate E2E
// suite can exercise the full intercept → request → consume → execute path
// without depending on a real provider. The leaf is annotated
// side_effect=true so the consumer's policy layer gates it exactly like a
// real mutating command. The definition (definitions/tools/gate-probe.json)
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
}

// Execute runs one gate-probe subcommand. env is ignored: the probe is
// credential-free by contract (design 318 — no auth block in its definition).
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
		Short:         "Approval-gate E2E probe (test-only)",
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
		Short:  "Approval-gate probe verbs",
		Hidden: true,
	}

	var note string
	send := &cobra.Command{
		Use:         "send",
		Short:       "Echo a local probe receipt (no-op, zero network)",
		Hidden:      true,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"}, // gated like a real mutating command (design 318)
		RunE: func(cmd *cobra.Command, _ []string) error {
			receipt := fmt.Sprintf(`{"tool":"gate-probe","action":"gate-probe.probe_send","status":"sent","note":%q}`, note)
			fmt.Fprintln(cmd.OutOrStdout(), receipt)
			return nil
		},
	}
	send.Flags().StringVar(&note, "note", "", "opaque marker echoed back in the receipt (lets tests vary argv)")

	probe.AddCommand(send)
	return probe
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
// traversal (tools.Service seam, design 318). The returned commands are never
// executed by Inspect/lint/policy consumers.
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot() }
