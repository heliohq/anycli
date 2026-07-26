package instantly

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newVerifyCmd(token string) *cobra.Command {
	cmd := newGroupCmd("verify", "Email verification (submit + poll)")
	cmd.AddCommand(
		s.newVerifyCreateCmd(token),
		s.newVerifyGetCmd(token),
	)
	return cmd
}

// newVerifyCreateCmd wraps POST /email-verification. Verification is async: the
// result may return status "pending" — poll `verify get --email` until done.
func (s *Service) newVerifyCreateCmd(token string) *cobra.Command {
	var email, webhookURL string
	cmd := &cobra.Command{
		Use:         "create",
		Annotations: writeAction,
		Short:       "Submit an email address for verification (POST /email-verification)",
		Long: "Asynchronous: the response can come back with status `pending`, which is\n" +
			"not a verdict — poll `verify get --email` until it resolves, or pass\n" +
			"--webhook-url to be told instead of polling. One address per call; there\n" +
			"is no bulk form here, so a whole list is verified in Instantly itself and\n" +
			"read back with `lead-list verification-stats`.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload := map[string]any{"email": email}
			if cmd.Flags().Changed("webhook-url") {
				payload["webhook_url"] = webhookURL
			}
			return s.send(cmd, token, http.MethodPost, "/email-verification", payload)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "email address to verify")
	cmd.Flags().StringVar(&webhookURL, "webhook-url", "", "webhook to notify on completion (optional)")
	_ = cmd.MarkFlagRequired("email")
	return cmd
}

func (s *Service) newVerifyGetCmd(token string) *cobra.Command {
	var email string
	cmd := &cobra.Command{
		Use:         "get",
		Annotations: readOnly,
		Short:       "Get a verification result (GET /email-verification/{email}); poll while pending",
		Long: "--email is required. This is the poll for `verify create`: a `pending`\n" +
			"status means the check is still running, not that the address is bad. It\n" +
			"only reads an existing result and starts nothing, so an address that was\n" +
			"never submitted has nothing to return here.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.get(cmd, token, "/email-verification/"+url.PathEscape(email), nil)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "email address to look up")
	_ = cmd.MarkFlagRequired("email")
	return cmd
}
