package sendgrid

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
)

func (s *Service) newContactCmd(token string, region *string) *cobra.Command {
	cmd := &cobra.Command{Use: "contact", Short: "Marketing contacts (upsert, search)"}
	cmd.AddCommand(
		s.newContactUpsertCmd(token, region),
		s.newContactSearchCmd(token, region),
	)
	return cmd
}

// newContactUpsertCmd wraps PUT /v3/marketing/contacts. The call is asynchronous
// and eventually consistent: it returns 202 with a JSON {job_id}; the contacts
// are queued, not yet stored. This surfaces the job_id verbatim — confirm with
// `contact search` (no silent "created" claim, DESIGN §2).
func (s *Service) newContactUpsertCmd(token string, region *string) *cobra.Command {
	var email, firstName, lastName, fullJSON string
	var customFields []string
	cmd := &cobra.Command{
		Use:         "upsert",
		Short:       "Add/update marketing contacts, async (PUT /v3/marketing/contacts). Returns job_id; confirm with `contact search`.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := buildContactPayload(email, firstName, lastName, customFields, fullJSON)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, *region, http.MethodPut, "/marketing/contacts", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "contact email (identifier)")
	cmd.Flags().StringVar(&firstName, "first-name", "", "contact first name")
	cmd.Flags().StringVar(&lastName, "last-name", "", "contact last name")
	cmd.Flags().StringArrayVar(&customFields, "custom-field", nil, "custom field key=value (repeatable)")
	cmd.Flags().StringVar(&fullJSON, "json-body", "", "full marketing contacts body JSON (escape hatch for bulk; overrides other flags)")
	return cmd
}

// buildContactPayload assembles the {contacts:[...]} upsert body from flags, or
// decodes the full-body escape hatch (which wins outright).
func buildContactPayload(email, firstName, lastName string, customFields []string, fullJSON string) (any, error) {
	if fullJSON != "" {
		return decodeJSONFlag("json-body", fullJSON)
	}
	if email == "" {
		return nil, fmt.Errorf("sendgrid: contact upsert requires --email (or --json-body)")
	}
	contact := map[string]any{"email": email}
	if firstName != "" {
		contact["first_name"] = firstName
	}
	if lastName != "" {
		contact["last_name"] = lastName
	}
	if len(customFields) > 0 {
		fields, err := parseCustomFields(customFields)
		if err != nil {
			return nil, err
		}
		contact["custom_fields"] = fields
	}
	return map[string]any{"contacts": []any{contact}}, nil
}

// parseCustomFields turns key=value pairs into a custom_fields object.
func parseCustomFields(pairs []string) (map[string]any, error) {
	fields := make(map[string]any, len(pairs))
	for _, pair := range pairs {
		key, value, ok := strings.Cut(pair, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("sendgrid: --custom-field %q must be key=value", pair)
		}
		fields[key] = value
	}
	return fields, nil
}

// newContactSearchCmd wraps POST /v3/marketing/contacts/search/emails: look up
// contacts by one or more email addresses.
func (s *Service) newContactSearchCmd(token string, region *string) *cobra.Command {
	var emails []string
	cmd := &cobra.Command{
		Use:         "search",
		Short:       "Look up contacts by email (POST /v3/marketing/contacts/search/emails)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if len(emails) == 0 {
				return fmt.Errorf("sendgrid: contact search requires at least one --email")
			}
			payload := map[string]any{"emails": emails}
			resp, err := s.call(cmd.Context(), token, *region, http.MethodPost, "/marketing/contacts/search/emails", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringArrayVar(&emails, "email", nil, "email to look up (repeatable)")
	return cmd
}
