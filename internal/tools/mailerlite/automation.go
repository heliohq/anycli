package mailerlite

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newAutomationCmd builds the `mailerlite automation` command tree. Automations
// are read-only via the API (no create): which flows exist and their run
// activity.
func (s *Service) newAutomationCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "automation", Short: "Automations (list, get, activity) — read-only"}
	cmd.AddCommand(
		s.newAutomationListCmd(token),
		s.newAutomationGetCmd(token),
		s.newAutomationActivityCmd(token),
	)
	return cmd
}

func (s *Service) newAutomationListCmd(token string) *cobra.Command {
	var limit, page int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List automations (GET /automations)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setLimitPage(cmd, q, limit, page)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/automations", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 25, "page size (default 25)")
	cmd.Flags().IntVar(&page, "page", 1, "page number (starts at 1)")
	return cmd
}

func (s *Service) newAutomationGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get an automation (GET /automations/{id})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/automations/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newAutomationActivityCmd(token string) *cobra.Command {
	var limit, page int
	cmd := &cobra.Command{
		Use:   "activity <id>",
		Short: "Automation subscriber activity (GET /automations/{id}/activity)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			setLimitPage(cmd, q, limit, page)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/automations/"+url.PathEscape(args[0])+"/activity", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 25, "page size (default 25)")
	cmd.Flags().IntVar(&page, "page", 1, "page number (starts at 1)")
	return cmd
}
