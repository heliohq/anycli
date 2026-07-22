package apollo

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newDealsCmd builds the `deals` group (Apollo opportunities): pipeline read +
// write. search / update are documented as master-API-key-gated and may return
// 403 to an OAuth token.
func (s *Service) newDealsCmd(token string) *cobra.Command {
	cmd := newGroupCmd("deals", "Manage deals (opportunities)")
	cmd.AddCommand(
		s.newDealsCreateCmd(token),
		s.newDealsSearchCmd(token),
		s.newDealsUpdateCmd(token),
	)
	return cmd
}

// newDealsCreateCmd wraps POST /opportunities.
func (s *Service) newDealsCreateCmd(token string) *cobra.Command {
	var body, name, ownerID, accountID, stageID string
	var amount string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a deal (POST /opportunities)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, err := bodyFromFlag(body)
			if err != nil {
				return err
			}
			setStr(b, "name", name)
			setStr(b, "owner_id", ownerID)
			setStr(b, "account_id", accountID)
			setStr(b, "opportunity_stage_id", stageID)
			setStr(b, "amount", amount)
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/opportunities", nil, b)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "deal name")
	cmd.Flags().StringVar(&ownerID, "owner-id", "", "deal owner user id")
	cmd.Flags().StringVar(&accountID, "account-id", "", "associated account id")
	cmd.Flags().StringVar(&stageID, "stage-id", "", "opportunity stage id")
	cmd.Flags().StringVar(&amount, "amount", "", "monetary amount")
	registerBodyFlag(cmd, &body)
	return cmd
}

// newDealsSearchCmd wraps GET /opportunities/search (master-API-key-gated).
func (s *Service) newDealsSearchCmd(token string) *cobra.Command {
	var page, perPage int
	cmd := &cobra.Command{
		Use:   "search",
		Short: "List/search deals (GET /opportunities/search)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			applyPageQuery(q, page, perPage)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/opportunities/search", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerPageFlags(cmd, &page, &perPage)
	return cmd
}

// newDealsUpdateCmd wraps PATCH /opportunities/{id} (master-API-key-gated).
func (s *Service) newDealsUpdateCmd(token string) *cobra.Command {
	var body, name, stageID, amount string
	cmd := &cobra.Command{
		Use:   "update <opportunity_id>",
		Short: "Update a deal (PATCH /opportunities/{id})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := bodyFromFlag(body)
			if err != nil {
				return err
			}
			setStr(b, "name", name)
			setStr(b, "opportunity_stage_id", stageID)
			setStr(b, "amount", amount)
			resp, err := s.call(cmd.Context(), token, http.MethodPatch, "/opportunities/"+url.PathEscape(args[0]), nil, b)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "deal name")
	cmd.Flags().StringVar(&stageID, "stage-id", "", "opportunity stage id")
	cmd.Flags().StringVar(&amount, "amount", "", "monetary amount")
	registerBodyFlag(cmd, &body)
	return cmd
}
