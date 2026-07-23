package zohocrm

import "github.com/spf13/cobra"

// NewCommandTree satisfies the design-318 Service interface: it returns the
// full cobra tree built with empty credentials (batch-reconciliation shim for
// a tool authored before NewCommandTree was added to the interface).
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot("") }
