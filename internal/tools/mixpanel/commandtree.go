package mixpanel

import "github.com/spf13/cobra"

// NewCommandTree satisfies the Service interface: it returns the
// full cobra tree built with empty credentials (batch-reconciliation shim).
// The returned commands are only traversed, never executed,
// so a zero-value credential/client is sufficient.
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot(&client{}) }
