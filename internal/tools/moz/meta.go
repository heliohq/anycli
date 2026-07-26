package moz

import (
	"github.com/spf13/cobra"
)

// newQuotaCmd looks up the account's remaining row quota for a metering path
// (quota.lookup). This call is free (costs zero quota) and a good habit before
// a large list pull, since every returned row debits the shared account quota.
func (s *Service) newQuotaCmd(token string) *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:   "quota",
		Short: "Remaining Moz API row quota (free check)",
		Long: "Costs no quota, so there is no reason to skip it before a large pull.\n" +
			"--path selects which meter to read and defaults to\n" +
			"api.limits.data.rows, the balance every data command here draws on; the\n" +
			"beta and mozscape paths are separate balances, and a healthy number on\n" +
			"one says nothing about the others.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			data := map[string]any{"path": path}
			result, err := s.call(cmd.Context(), token, "quota.lookup", data)
			if err != nil {
				return err
			}
			return s.emit(result)
		},
	}
	cmd.Flags().StringVar(&path, "path", "api.limits.data.rows", "quota path: api.limits.data.rows|api.limits.beta.rows|api.limits.mozscape.rows")
	return cmd
}

// newIndexCmd reports the current Moz index freshness metadata
// (metadata.index.fetch). This call is free.
func (s *Service) newIndexCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "index",
		Short: "Moz link-index freshness metadata (free check)",
		Long: "Costs no quota. Reports when Moz last rebuilt its link index, which\n" +
			"bounds how fresh any link or authority number from this tool can\n" +
			"possibly be — a backlink won since that date cannot appear yet. Check it\n" +
			"before concluding that a known link is missing.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := s.call(cmd.Context(), token, "metadata.index.fetch", map[string]any{})
			if err != nil {
				return err
			}
			return s.emit(result)
		},
	}
}
