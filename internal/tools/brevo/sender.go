package brevo

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// itoa renders an int as a base-10 string for path segments and query values.
func itoa(n int) string { return strconv.Itoa(n) }

// newSenderLsCmd builds `brevo sender ls` — GET /senders. Brevo blocks sends
// from unverified senders, so this is the discovery verb an agent uses to pick
// a verified sender before `email send` / `campaign create`.
func (s *Service) newSenderLsCmd(apiKey string) *cobra.Command {
	var ip, domain string
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List verified senders (GET /senders)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if ip != "" {
				q.Set("ip", ip)
			}
			if domain != "" {
				q.Set("domain", domain)
			}
			resp, err := s.call(cmd.Context(), apiKey, http.MethodGet, "/senders", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&ip, "ip", "", "filter senders by dedicated IP (optional)")
	cmd.Flags().StringVar(&domain, "domain", "", "filter senders by domain (optional)")
	return cmd
}

// newAccountGetCmd builds `brevo account get` — GET /account. Returns the
// account identity (login email, company name), plan, and credits. Doubles as
// the api_key verification endpoint (a bad key returns 401 unauthorized).
func (s *Service) newAccountGetCmd(apiKey string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get account identity, plan, and credits (GET /account)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), apiKey, http.MethodGet, "/account", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}
