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
		Long: "Page-numbered with --page. Automations are read-only over this API: they\n" +
			"cannot be created, edited, enabled, disabled or deleted here, and no\n" +
			"command enrolls a subscriber into one. Enrollment happens through the\n" +
			"automation's own trigger, so the lever available from here is usually\n" +
			"`group assign`.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
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
		Long: "The flow's configuration — its trigger, its steps and whether it is\n" +
			"currently enabled. Read-only: the enabled state shown here cannot be\n" +
			"changed through this API.",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
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
		Long: "Which subscribers actually moved through this automation and where they\n" +
			"are in it — the answer to \"did the welcome flow fire for this person\",\n" +
			"from the flow's side. Page-numbered with --page. The same question from\n" +
			"the subscriber's side is `subscriber activity`.",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
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
