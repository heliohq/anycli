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
		Use:   "create",
		Short: "Submit an email address for verification (POST /email-verification)",
		Args:  cobra.NoArgs,
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
		Use:   "get",
		Short: "Get a verification result (GET /email-verification/{email}); poll while pending",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.get(cmd, token, "/email-verification/"+url.PathEscape(email), nil)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "email address to look up")
	_ = cmd.MarkFlagRequired("email")
	return cmd
}
