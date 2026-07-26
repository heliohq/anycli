package mailjet

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newContactCmd groups contact read/create over /v3/REST/contact.
func (s *Service) newContactCmd(basic string) *cobra.Command {
	cmd := newGroupCmd("contact", "Manage contacts (list, get, create)")
	cmd.AddCommand(
		s.newContactListCmd(basic),
		s.newContactGetCmd(basic),
		s.newContactCreateCmd(basic),
	)
	return cmd
}

func (s *Service) newContactListCmd(basic string) *cobra.Command {
	var limit, offset int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List contacts (GET /v3/REST/contact)",
		Long: "The account-wide address book, not the membership of any one list — a contact\n" +
			"exists independently of every contact list and can belong to none. `--limit`\n" +
			"defaults to 10 and Mailjet caps it at 1000, so walking a real address book\n" +
			"means stepping `--offset`. Each row's `ID` is what `list add-contact\n" +
			"--contact-id` requires.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			baseURL, err := s.resolveBaseURL(cmd)
			if err != nil {
				return err
			}
			q := url.Values{}
			q.Set("Limit", itoa(limit))
			q.Set("Offset", itoa(offset))
			resp, err := s.call(cmd.Context(), basic, baseURL, http.MethodGet, "/v3/REST/contact", q, nil)
			if err != nil {
				return err
			}
			return s.emitList(resp)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 10, "max contacts to return (Mailjet caps at 1000)")
	cmd.Flags().IntVar(&offset, "offset", 0, "pagination offset")
	return cmd
}

func (s *Service) newContactGetCmd(basic string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get one contact by ID or email (GET /v3/REST/contact/{id})",
		Long: "`--id` is required and accepts EITHER the numeric contact id or the email\n" +
			"address itself, so an address needs no prior lookup. Returns the single record\n" +
			"rather than a one-element array. This is also the cheapest membership test: an\n" +
			"address Mailjet has never seen simply does not resolve.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			baseURL, err := s.resolveBaseURL(cmd)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), basic, baseURL, http.MethodGet, "/v3/REST/contact/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			return s.emitOne(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "contact ID or email address")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newContactCreateCmd(basic string) *cobra.Command {
	var email, name string
	var excluded bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a contact (POST /v3/REST/contact)",
		Long: "`--email` is required. Creating a contact adds it to the address book only —\n" +
			"it joins no list and receives nothing until `list add-contact` puts it on one.\n" +
			"`--excluded` sets Mailjet's campaign exclusion flag and is sent only when the\n" +
			"flag is actually passed, so omitting it leaves Mailjet's default rather than\n" +
			"forcing `false`. Mailjet rejects an email that already exists, so this is not\n" +
			"an upsert.",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			baseURL, err := s.resolveBaseURL(cmd)
			if err != nil {
				return err
			}
			body := map[string]any{"Email": email}
			if name != "" {
				body["Name"] = name
			}
			if cmd.Flags().Changed("excluded") {
				body["IsExcludedFromCampaigns"] = excluded
			}
			resp, err := s.call(cmd.Context(), basic, baseURL, http.MethodPost, "/v3/REST/contact", nil, body)
			if err != nil {
				return err
			}
			return s.emitOne(resp)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "contact email address")
	cmd.Flags().StringVar(&name, "name", "", "contact display name (optional)")
	cmd.Flags().BoolVar(&excluded, "excluded", false, "exclude the contact from all campaigns")
	_ = cmd.MarkFlagRequired("email")
	return cmd
}
