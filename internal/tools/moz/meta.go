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
		Use:         "quota",
		Short:       "Remaining Moz API row quota (free check)",
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
		Use:         "index",
		Short:       "Moz link-index freshness metadata (free check)",
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
