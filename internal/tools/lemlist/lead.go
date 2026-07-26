package lemlist

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newLeadCmd groups lead enrollment, lookup, update, and disposition verbs.
func (s *Service) newLeadCmd(key string) *cobra.Command {
	cmd := newGroupCmd("lead", "Leads: add, get, update, unsubscribe, delete, mark disposition")
	cmd.AddCommand(
		s.newLeadAddCmd(key),
		s.newLeadGetCmd(key),
		s.newLeadUpdateCmd(key),
		s.newLeadUnsubscribeCmd(key),
		s.newLeadDeleteCmd(key),
		s.newLeadMarkCmd(key, "mark-interested", "interested", longLeadInterested),
		s.newLeadMarkCmd(key, "mark-not-interested", "notinterested", longLeadNotInterested),
	)
	return cmd
}

func (s *Service) newLeadAddCmd(key string) *cobra.Command {
	var email, firstName, lastName, companyName, jobTitle, linkedinURL, phone, fieldsJSON string
	cmd := &cobra.Command{
		Use:   "add <campaignId>",
		Short: "Enroll a lead into a campaign (POST /campaigns/{campaignId}/leads/)",
		Long: "`--email` is required and is the lead's identity. `--first-name`,\n" +
			"`--last-name`, `--company-name`, `--job-title`, `--linkedin-url` and\n" +
			"`--phone` have flags; everything else, custom variables included, goes\n" +
			"into `--fields` as a JSON object, and a named flag overwrites the\n" +
			"matching key there. Those variables are what the sequence's templates\n" +
			"interpolate, so a missing one leaves a visible gap in a real email.\n" +
			"Enrolment does not start a campaign — but adding a lead to one that is\n" +
			"already running queues it for sending straight away.",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{}
			if fieldsJSON != "" {
				parsed, err := decodeJSONFlag("fields", fieldsJSON)
				if err != nil {
					return err
				}
				payload = parsed
			}
			payload["email"] = email
			setIfNotEmpty(payload, "firstName", firstName)
			setIfNotEmpty(payload, "lastName", lastName)
			setIfNotEmpty(payload, "companyName", companyName)
			setIfNotEmpty(payload, "jobTitle", jobTitle)
			setIfNotEmpty(payload, "linkedinUrl", linkedinURL)
			setIfNotEmpty(payload, "phone", phone)

			path := "/campaigns/" + url.PathEscape(args[0]) + "/leads/"
			body, err := s.call(cmd.Context(), key, http.MethodPost, path, nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "lead email (required)")
	cmd.Flags().StringVar(&firstName, "first-name", "", "lead first name")
	cmd.Flags().StringVar(&lastName, "last-name", "", "lead last name")
	cmd.Flags().StringVar(&companyName, "company-name", "", "lead company name")
	cmd.Flags().StringVar(&jobTitle, "job-title", "", "lead job title")
	cmd.Flags().StringVar(&linkedinURL, "linkedin-url", "", "lead LinkedIn profile URL")
	cmd.Flags().StringVar(&phone, "phone", "", "lead phone number")
	cmd.Flags().StringVar(&fieldsJSON, "fields", "", "additional lead fields / custom variables as a JSON object")
	_ = cmd.MarkFlagRequired("email")
	return cmd
}

func (s *Service) newLeadGetCmd(key string) *cobra.Command {
	var email, id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Look up a lead by email or id (GET /leads)",
		Long: "Account-wide rather than campaign-scoped, and needs `--email` or `--id`;\n" +
			"with neither it fails before any request is made. This is how an\n" +
			"address becomes the lead id that `lead update` and `lead delete`\n" +
			"insist on, and how you see which campaigns a person is already sitting\n" +
			"in before enrolling them again.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if email == "" && id == "" {
				return &usageError{msg: "lemlist: lead get requires --email or --id"}
			}
			q := url.Values{}
			q.Set("version", "v2")
			if email != "" {
				q.Set("email", email)
			}
			if id != "" {
				q.Set("id", id)
			}
			body, err := s.call(cmd.Context(), key, http.MethodGet, "/leads", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "lead email (use --email or --id)")
	cmd.Flags().StringVar(&id, "id", "", "lead id (use --email or --id)")
	return cmd
}

func (s *Service) newLeadUpdateCmd(key string) *cobra.Command {
	var fieldsJSON string
	cmd := &cobra.Command{
		Use:   "update <campaignId> <leadId>",
		Short: "Update a lead's fields in a campaign (PATCH /campaigns/{campaignId}/leads/{leadId})",
		Long: "Campaign id first, then the LEAD id — from `lead get`, not an email.\n" +
			"`--fields` is required and is a JSON object of the fields to change;\n" +
			"only the keys sent are touched. The change is scoped to this campaign,\n" +
			"so the same person enrolled elsewhere keeps their old values there,\n" +
			"including the custom variables their other sequences interpolate.",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := decodeJSONFlag("fields", fieldsJSON)
			if err != nil {
				return err
			}
			path := "/campaigns/" + url.PathEscape(args[0]) + "/leads/" + url.PathEscape(args[1])
			body, err := s.call(cmd.Context(), key, http.MethodPatch, path, nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&fieldsJSON, "fields", "", "lead fields to update as a JSON object (required)")
	_ = cmd.MarkFlagRequired("fields")
	return cmd
}

func (s *Service) newLeadUnsubscribeCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:   "unsubscribe <campaignId> <email>",
		Short: "Unsubscribe a lead from a campaign (DELETE /campaigns/{campaignId}/leads/{email})",
		Long: "Takes the campaign id and the lead's EMAIL, not a lead id. The lead\n" +
			"stops receiving this campaign's steps but keeps its record and\n" +
			"history, which is the whole difference from `lead delete`. It\n" +
			"suppresses within this campaign only — account-wide suppression, or a\n" +
			"whole domain, is `unsubscribe add`.",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "/campaigns/" + url.PathEscape(args[0]) + "/leads/" + url.PathEscape(args[1])
			body, err := s.call(cmd.Context(), key, http.MethodDelete, path, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newLeadDeleteCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <campaignId> <leadId>",
		Short: "Force-delete a lead by id from a campaign (DELETE /campaigns/{campaignId}/leads/{leadId}?action=remove)",
		Long: "Takes the campaign id and the LEAD ID, not an email, and always sends\n" +
			"`action=remove` so the lead is genuinely deleted — without that\n" +
			"parameter Lemlist quietly downgrades the call to an unsubscribe and\n" +
			"then expects an email instead. The lead and its history leave the\n" +
			"campaign and nothing here restores them. When the intent is only to\n" +
			"stop contacting someone, `lead unsubscribe` keeps the record.",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Lemlist only force-deletes when action=remove is sent; without it
			// the endpoint silently falls back to unsubscribing (and then expects
			// an email, not a lead id). Always send action=remove so `delete`
			// actually removes the lead.
			q := url.Values{"action": {"remove"}}
			path := "/campaigns/" + url.PathEscape(args[0]) + "/leads/" + url.PathEscape(args[1])
			body, err := s.call(cmd.Context(), key, http.MethodDelete, path, q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// longLeadInterested and longLeadNotInterested are the two disposition Longs.
// They live next to the shared builder because it is the builder that fixes
// the endpoint and the id-or-email argument both of them describe.
const (
	longLeadInterested = "Records the lead as interested, account-wide rather than inside one\n" +
		"campaign, and accepts either a lead id or an email. This is the\n" +
		"disposition to set after a positive reply, and what makes the lead count\n" +
		"as an outcome of the outreach rather than another open. It does not stop\n" +
		"the sequence: unless the campaign is configured to halt on reply, the\n" +
		"lead keeps receiving steps until it is unsubscribed."

	longLeadNotInterested = "Records the lead as not interested, account-wide, from either a lead id\n" +
		"or an email. It marks the outcome and suppresses nobody — someone who\n" +
		"asked not to be contacted still needs `lead unsubscribe` for that\n" +
		"campaign, or `unsubscribe add` for the whole account. On its own this\n" +
		"leaves the sequence running."
)

// newLeadMarkCmd builds `mark-interested` / `mark-not-interested`, which POST a
// disposition against a lead id or email
// (POST /leads/{interested|notinterested}/{leadIdOrEmail}).
func (s *Service) newLeadMarkCmd(key, use, verb, long string) *cobra.Command {
	return &cobra.Command{
		Use:         use + " <leadIdOrEmail>",
		Short:       "Set the lead pipeline disposition (POST /leads/" + verb + "/{leadIdOrEmail})",
		Long:        long,
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "/leads/" + verb + "/" + url.PathEscape(args[0])
			body, err := s.call(cmd.Context(), key, http.MethodPost, path, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// setIfNotEmpty writes v into m under k only when v is non-empty, so named
// flags never clobber a value already supplied via --fields with an empty
// string.
func setIfNotEmpty(m map[string]any, k, v string) {
	if v != "" {
		m[k] = v
	}
}
