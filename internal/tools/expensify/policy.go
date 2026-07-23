package expensify

import "github.com/spf13/cobra"

func (s *Service) newPolicyCmd(creds credentials) *cobra.Command {
	cmd := &cobra.Command{Use: "policy", Short: "Expensify policies (list, get)"}
	cmd.AddCommand(
		s.newPolicyListCmd(creds),
		s.newPolicyGetCmd(creds),
	)
	return cmd
}

func (s *Service) newPolicyListCmd(creds credentials) *cobra.Command {
	var adminOnly bool
	var userEmail string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List policies (get / policyList)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // read-only get
		RunE: func(cmd *cobra.Command, _ []string) error {
			in := map[string]any{"type": "policyList"}
			if adminOnly {
				in["adminOnly"] = true
			}
			if userEmail != "" {
				in["userEmail"] = userEmail
			}
			body, err := s.call(cmd.Context(), creds, map[string]any{"type": "get", "inputSettings": in})
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().BoolVar(&adminOnly, "admin-only", false, "only policies the user administers")
	cmd.Flags().StringVar(&userEmail, "user-email", "", "act on behalf of this employee email (optional)")
	return cmd
}

func (s *Service) newPolicyGetCmd(creds credentials) *cobra.Command {
	var policyIDs, fields []string
	var userEmail string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get policy details (get / policy)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // read-only get
		RunE: func(cmd *cobra.Command, _ []string) error {
			in := map[string]any{"type": "policy", "policyIDList": policyIDs}
			if len(fields) > 0 {
				in["fields"] = fields
			}
			if userEmail != "" {
				in["userEmail"] = userEmail
			}
			body, err := s.call(cmd.Context(), creds, map[string]any{"type": "get", "inputSettings": in})
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringArrayVar(&policyIDs, "policy-id", nil, "policy ID (repeatable, required)")
	cmd.Flags().StringArrayVar(&fields, "field", nil, "detail field: categories|reportFields|tags|tax|employees (repeatable)")
	cmd.Flags().StringVar(&userEmail, "user-email", "", "act on behalf of this employee email (optional)")
	_ = cmd.MarkFlagRequired("policy-id")
	return cmd
}
