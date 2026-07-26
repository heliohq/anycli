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
		Use:         "list",
		Annotations: readOnly,
		Short:       "List sending accounts (GET /accounts)",
		Long: "Sending accounts are addressed by EMAIL everywhere else in this group, and\n" +
			"this is where those addresses come from. --search matches the email as a\n" +
			"substring, --status takes Instantly's numeric account status code, and\n" +
			"--provider-code the email provider. Cursor-paged with --limit and\n" +
			"--starting-after.",
		Args: cobra.NoArgs,
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
		Use:         "get",
		Annotations: readOnly,
		Short:       "Get a sending account (GET /accounts/{email})",
		Long: "Addressed by the account's --email, not by an id. Returns that sender's\n" +
			"configuration and health, including its warmup settings and daily sending\n" +
			"limit — the number that decides how much of a campaign this account can\n" +
			"actually carry.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.get(cmd, token, "/accounts/"+url.PathEscape(email), nil)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "sending account email")
	_ = cmd.MarkFlagRequired("email")
	return cmd
}

// longAccountPause and longAccountResume are the two sender-rotation Longs.
// They sit next to the shared builder because it is the builder that fixes the
// bodyless POST on a single --email both describe, while the consequence — the
// remaining senders absorbing the volume, versus a sender re-entering rotation
// — differs per direction.
const (
	longAccountPause = "Takes the account's --email and takes that sender out of rotation for every\n" +
		"campaign it is attached to, without changing the campaigns themselves. The\n" +
		"remaining senders carry the same volume, so their per-account rate rises.\n" +
		"This is how a sender whose deliverability is degrading is isolated;\n" +
		"`account resume` reverses it."

	longAccountResume = "Takes the account's --email and puts the sender back into rotation for the\n" +
		"campaigns it is attached to. It does not restart a paused campaign, which\n" +
		"is `campaign activate`. Check the sender's daily limit with `account get`\n" +
		"before assuming it can absorb a backlog."
)

func (s *Service) newAccountPauseCmd(token string) *cobra.Command {
	return s.accountAction(token, "pause", "Pause a sending account (POST /accounts/{email}/pause)", longAccountPause, "/pause")
}

func (s *Service) newAccountResumeCmd(token string) *cobra.Command {
	return s.accountAction(token, "resume", "Resume a sending account (POST /accounts/{email}/resume)", longAccountResume, "/resume")
}

// accountAction builds a no-body POST action on a single account email.
func (s *Service) accountAction(token, use, short, long, suffix string) *cobra.Command {
	var email string
	cmd := &cobra.Command{
		Use:         use,
		Annotations: writeAction,
		Short:       short,
		Long:        long,
		Args:        cobra.NoArgs,
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
		Use:         "warmup-analytics",
		Annotations: readOnly,
		Short:       "Warmup analytics for accounts (POST /accounts/warmup-analytics)",
		Long: "A POST whose body is the required comma-separated --emails list, so it\n" +
			"covers only the accounts named — there is no workspace-wide form. Warmup\n" +
			"is the automated inbox-to-inbox traffic that builds a sender's\n" +
			"reputation, so these numbers are about deliverability health and say\n" +
			"nothing about campaign performance.",
		Args: cobra.NoArgs,
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
		Use:         "analytics-daily",
		Annotations: readOnly,
		Short:       "Daily sending analytics (GET /accounts/analytics/daily)",
		Long: "Per-day figures for the SENDING ACCOUNTS rather than for a campaign —\n" +
			"`campaign analytics-daily` is the campaign view of the same period, and\n" +
			"the two answer different questions when one sender is degrading. --emails\n" +
			"is a comma-separated list of accounts and narrows it; omitting it covers\n" +
			"the whole workspace. --start-date and --end-date are YYYY-MM-DD.",
		Args: cobra.NoArgs,
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
