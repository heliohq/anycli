package instantly

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

func (s *Service) newAccountCmd(token string) *cobra.Command {
	cmd := newGroupCmd("account", "Sending accounts (health, warmup, deliverability)")
	cmd.AddCommand(
		s.newAccountListCmd(token),
		s.newAccountGetCmd(token),
		s.newAccountPauseCmd(token),
		s.newAccountResumeCmd(token),
		s.newAccountWarmupAnalyticsCmd(token),
		s.newAccountAnalyticsDailyCmd(token),
	)
	return cmd
}

func (s *Service) newAccountListCmd(token string) *cobra.Command {
	var page pageFlags
	var search, status, providerCode, tagIDs string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List sending accounts (GET /accounts)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			page.applyQuery(q)
			setIfChanged(cmd, q, "search", "search", search)
			setIfChanged(cmd, q, "status", "status", status)
			setIfChanged(cmd, q, "provider-code", "provider_code", providerCode)
			setIfChanged(cmd, q, "tag-ids", "tag_ids", tagIDs)
			return s.get(cmd, token, "/accounts", q)
		},
	}
	registerPageFlags(cmd, &page)
	cmd.Flags().StringVar(&search, "search", "", "filter by email substring")
	cmd.Flags().StringVar(&status, "status", "", "filter by account status code")
	cmd.Flags().StringVar(&providerCode, "provider-code", "", "filter by email provider code")
	cmd.Flags().StringVar(&tagIDs, "tag-ids", "", "comma-separated tag ids")
	return cmd
}

func (s *Service) newAccountGetCmd(token string) *cobra.Command {
	var email string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a sending account (GET /accounts/{email})",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.get(cmd, token, "/accounts/"+url.PathEscape(email), nil)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "sending account email")
	_ = cmd.MarkFlagRequired("email")
	return cmd
}

func (s *Service) newAccountPauseCmd(token string) *cobra.Command {
	return s.accountAction(token, "pause", "Pause a sending account (POST /accounts/{email}/pause)", "/pause")
}

func (s *Service) newAccountResumeCmd(token string) *cobra.Command {
	return s.accountAction(token, "resume", "Resume a sending account (POST /accounts/{email}/resume)", "/resume")
}

// accountAction builds a no-body POST action on a single account email.
func (s *Service) accountAction(token, use, short, suffix string) *cobra.Command {
	var email string
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.send(cmd, token, http.MethodPost, "/accounts/"+url.PathEscape(email)+suffix, nil)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "sending account email")
	_ = cmd.MarkFlagRequired("email")
	return cmd
}

// newAccountWarmupAnalyticsCmd wraps POST /accounts/warmup-analytics — the body
// is {"emails":[...]} (required).
func (s *Service) newAccountWarmupAnalyticsCmd(token string) *cobra.Command {
	var emails string
	cmd := &cobra.Command{
		Use:   "warmup-analytics",
		Short: "Warmup analytics for accounts (POST /accounts/warmup-analytics)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload := map[string]any{"emails": splitCSV(emails)}
			return s.send(cmd, token, http.MethodPost, "/accounts/warmup-analytics", payload)
		},
	}
	cmd.Flags().StringVar(&emails, "emails", "", "comma-separated sending account emails")
	_ = cmd.MarkFlagRequired("emails")
	return cmd
}

func (s *Service) newAccountAnalyticsDailyCmd(token string) *cobra.Command {
	var startDate, endDate, emails string
	cmd := &cobra.Command{
		Use:   "analytics-daily",
		Short: "Daily sending analytics (GET /accounts/analytics/daily)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setIfChanged(cmd, q, "start-date", "start_date", startDate)
			setIfChanged(cmd, q, "end-date", "end_date", endDate)
			setIfChanged(cmd, q, "emails", "emails", emails)
			return s.get(cmd, token, "/accounts/analytics/daily", q)
		},
	}
	registerAnalyticsRangeFlags(cmd, &startDate, &endDate)
	cmd.Flags().StringVar(&emails, "emails", "", "comma-separated sending account emails")
	return cmd
}

// splitCSV splits a comma-separated flag value into a trimmed, non-empty slice.
func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
