package hunter

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newEmailFinderCmd wraps GET /email-finder: the most likely email for a named
// person at a domain/company, or via a LinkedIn handle. Costs 1 credit.
func (s *Service) newEmailFinderCmd(key string) *cobra.Command {
	var domain, company, firstName, lastName, fullName, linkedinHandle string
	var maxDuration int
	cmd := &cobra.Command{
		Use:   "email-finder",
		Short: "Find a person's email address (GET /email-finder)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setIf(q, "domain", domain)
			setIf(q, "company", company)
			setIf(q, "first_name", firstName)
			setIf(q, "last_name", lastName)
			setIf(q, "full_name", fullName)
			setIf(q, "linkedin_handle", linkedinHandle)
			if cmd.Flags().Changed("max-duration") {
				q.Set("max_duration", itoa(maxDuration))
			}
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/email-finder", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "", "domain (e.g. stripe.com)")
	cmd.Flags().StringVar(&company, "company", "", "company name (alternative to --domain)")
	cmd.Flags().StringVar(&firstName, "first-name", "", "person's first name")
	cmd.Flags().StringVar(&lastName, "last-name", "", "person's last name")
	cmd.Flags().StringVar(&fullName, "full-name", "", "person's full name (alternative to first/last)")
	cmd.Flags().StringVar(&linkedinHandle, "linkedin-handle", "", "LinkedIn handle (alternative to name+domain)")
	cmd.Flags().IntVar(&maxDuration, "max-duration", 0, "max seconds to spend (3-20, default 10)")
	return cmd
}
