package delighted

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newBouncesCmd wires `delighted bounces list` — GET /bounces.json, people whose
// survey email bounced.
func (s *Service) newBouncesCmd(key string) *cobra.Command {
	cmd := &cobra.Command{Use: "bounces", Short: "Bounced survey recipients"}
	list := &cobra.Command{
		Use:   "list",
		Short: "List bounced people (GET /bounces.json)",
		Long: "Recipients whose survey email hard-bounced, which is why a send can look\n" +
			"successful yet produce no response — check here before concluding someone\n" +
			"ignored a survey. Bounces are recorded by the provider, not set here, and\n" +
			"there is no verb that clears one; fixing the address means a fresh\n" +
			"`people send`. Page with `--per-page` and `--page`.",
		Args: cobra.NoArgs,
	}
	list.Annotations = readOnly
	perPage, page := registerPaging(list)
	list.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		applyPaging(q, *perPage, *page)
		resp, err := s.call(cmd.Context(), key, http.MethodGet, "/bounces.json", q, nil)
		if err != nil {
			return err
		}
		return s.emit(resp)
	}
	cmd.AddCommand(list)
	return cmd
}

// newUnsubscribesCmd wires the unsubscribe resource: list existing unsubscribes
// and add a new one, over /unsubscribes(.json).
func (s *Service) newUnsubscribesCmd(key string) *cobra.Command {
	cmd := &cobra.Command{Use: "unsubscribes", Short: "Unsubscribed recipients"}
	cmd.AddCommand(
		s.newUnsubscribesListCmd(key),
		s.newUnsubscribesAddCmd(key),
	)
	return cmd
}

func (s *Service) newUnsubscribesListCmd(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List unsubscribed people (GET /unsubscribes.json)",
		Long: "The suppression list: people who will never receive another survey,\n" +
			"whether they opted out themselves or were added by `unsubscribes add`.\n" +
			"Check it before a send campaign, since a suppressed address consumes the\n" +
			"request and delivers nothing. Page with `--per-page` and `--page`.",
		Args: cobra.NoArgs,
	}
	cmd.Annotations = readOnly
	perPage, page := registerPaging(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		applyPaging(q, *perPage, *page)
		resp, err := s.call(cmd.Context(), key, http.MethodGet, "/unsubscribes.json", q, nil)
		if err != nil {
			return err
		}
		return s.emit(resp)
	}
	return cmd
}

func (s *Service) newUnsubscribesAddCmd(key string) *cobra.Command {
	var personEmail string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Unsubscribe a person (POST /unsubscribes.json)",
		Long: "Suppresses `--person-email` permanently: no future survey reaches that\n" +
			"address, including one an Autopilot enrollment would have sent. There is\n" +
			"no matching remove verb, so this is one-way from this tool — re-enabling\n" +
			"someone is a manual step in the Delighted UI. Existing responses are kept,\n" +
			"unlike `people delete`.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload := map[string]any{"person_email": personEmail}
			resp, err := s.call(cmd.Context(), key, http.MethodPost, "/unsubscribes.json", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&personEmail, "person-email", "", "email of the person to unsubscribe")
	_ = cmd.MarkFlagRequired("person-email")
	return cmd
}
