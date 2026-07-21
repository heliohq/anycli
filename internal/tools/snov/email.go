package snov

import (
	"net/http"
	"net/url"
	"time"

	"github.com/spf13/cobra"
)

// newEmailCmd groups the email finder + verifier surface — the core of what a
// sales assistant does with Snov: find a company's or a person's business
// email, count how many a domain has (free), and verify deliverability before
// sending. The finder and verifier are asynchronous Snov v2 tasks; each command
// blocks on the start→poll→result loop and emits only the finished payload.
func (s *Service) newEmailCmd(creds clientCreds) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "email",
		Short: "Find, count, and verify email addresses",
	}
	cmd.AddCommand(
		s.newEmailFindCmd(creds),
		s.newEmailVerifyCmd(creds),
		s.newEmailCountCmd(creds),
	)
	return cmd
}

func (s *Service) newEmailFindCmd(creds clientCreds) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "find",
		Short: "Find email addresses (consumes credits)",
	}
	cmd.AddCommand(
		s.newEmailFindDomainCmd(creds),
		s.newEmailFindByNameCmd(creds),
	)
	return cmd
}

func (s *Service) newEmailFindDomainCmd(creds clientCreds) *cobra.Command {
	var domain string
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "domain",
		Short: "Find all business emails for a company domain (consumes credits)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.startAndPoll(cmd.Context(), creds,
				"/v2/domain-search/domain-emails/start",
				"/v2/domain-search/domain-emails/result",
				map[string]any{"domain": domain},
				s.effectiveTimeout(timeout))
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "", "company domain, e.g. example.com (required)")
	registerTimeoutFlag(cmd, &timeout)
	_ = cmd.MarkFlagRequired("domain")
	return cmd
}

func (s *Service) newEmailFindByNameCmd(creds clientCreds) *cobra.Command {
	var first, last, domain string
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "by-name",
		Short: "Find a specific person's email from their name and company domain (consumes credits)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload := map[string]any{"rows": []map[string]string{{
				"first_name": first,
				"last_name":  last,
				"domain":     domain,
			}}}
			body, err := s.startAndPoll(cmd.Context(), creds,
				"/v2/emails-by-domain-by-name/start",
				"/v2/emails-by-domain-by-name/result",
				payload,
				s.effectiveTimeout(timeout))
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&first, "first", "", "person's first name (required)")
	cmd.Flags().StringVar(&last, "last", "", "person's last name (required)")
	cmd.Flags().StringVar(&domain, "domain", "", "company domain, e.g. example.com (required)")
	registerTimeoutFlag(cmd, &timeout)
	_ = cmd.MarkFlagRequired("first")
	_ = cmd.MarkFlagRequired("last")
	_ = cmd.MarkFlagRequired("domain")
	return cmd
}

func (s *Service) newEmailVerifyCmd(creds clientCreds) *cobra.Command {
	var emails []string
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify email deliverability before sending (consumes credits)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.startAndPoll(cmd.Context(), creds,
				"/v2/email-verification/start",
				"/v2/email-verification/result",
				map[string]any{"emails": emails},
				s.effectiveTimeout(timeout))
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringArrayVar(&emails, "email", nil, "email address to verify; repeat for up to 10 (required)")
	registerTimeoutFlag(cmd, &timeout)
	_ = cmd.MarkFlagRequired("email")
	return cmd
}

func (s *Service) newEmailCountCmd(creds clientCreds) *cobra.Command {
	var domain string
	cmd := &cobra.Command{
		Use:   "count",
		Short: "Count how many emails Snov has for a domain (free)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			params := url.Values{}
			params.Set("domain", domain)
			body, err := s.callV1(cmd.Context(), creds, http.MethodPost, "/v1/get-domain-emails-count", params)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "", "company domain, e.g. example.com (required)")
	_ = cmd.MarkFlagRequired("domain")
	return cmd
}

// registerTimeoutFlag wires the async --timeout flag (0 = the service default).
func registerTimeoutFlag(cmd *cobra.Command, timeout *time.Duration) {
	cmd.Flags().DurationVar(timeout, "timeout", 0, "max time to wait for the async task (e.g. 90s; 0 = default)")
}

// effectiveTimeout resolves the per-command --timeout flag against the service
// default.
func (s *Service) effectiveTimeout(flag time.Duration) time.Duration {
	if flag > 0 {
		return flag
	}
	return s.pollTimeout()
}
