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
		s.newLeadMarkCmd(key, "mark-interested", "interested"),
		s.newLeadMarkCmd(key, "mark-not-interested", "notinterested"),
	)
	return cmd
}

func (s *Service) newLeadAddCmd(key string) *cobra.Command {
	var email, firstName, lastName, companyName, jobTitle, linkedinURL, phone, fieldsJSON string
	cmd := &cobra.Command{
		Use:   "add <campaignId>",
		Short: "Enroll a lead into a campaign (POST /campaigns/{campaignId}/leads/)",
		Args:  cobra.ExactArgs(1),
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
		Args:  cobra.NoArgs,
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
		Args:  cobra.ExactArgs(2),
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
		Args:  cobra.ExactArgs(2),
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
		Short: "Delete (or unsubscribe) a lead by id (DELETE /campaigns/{campaignId}/leads/{leadId})",
		Args:  cobra.ExactArgs(2),
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

// newLeadMarkCmd builds `mark-interested` / `mark-not-interested`, which POST a
// disposition against a lead id or email
// (POST /leads/{interested|notinterested}/{leadIdOrEmail}).
func (s *Service) newLeadMarkCmd(key, use, verb string) *cobra.Command {
	return &cobra.Command{
		Use:   use + " <leadIdOrEmail>",
		Short: "Set the lead pipeline disposition (POST /leads/" + verb + "/{leadIdOrEmail})",
		Args:  cobra.ExactArgs(1),
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
