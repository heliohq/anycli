package courier

import (
	"net/http"

	"github.com/spf13/cobra"
)

// sendFlags holds the recipient / content / options flags for `send`.
type sendFlags struct {
	userID     string
	email      string
	phone      string
	listID     string
	audienceID string

	template string
	title    string
	body     string

	data    string
	routing string
	brandID string
}

// newSendCmd builds `courier send` — the core action. Exactly one recipient
// selector and exactly one content form (template XOR title+body) are required;
// violations are usage errors (exit 2) surfaced before any HTTP call.
func (s *Service) newSendCmd(key string) *cobra.Command {
	var f sendFlags
	cmd := &cobra.Command{
		Use:         "send",
		Short:       "Send a notification to a recipient across configured channels",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			to, err := f.recipient()
			if err != nil {
				return err
			}
			content, err := f.content()
			if err != nil {
				return err
			}
			message := map[string]any{"to": to}
			for k, v := range content {
				message[k] = v
			}
			if f.data != "" {
				d, err := decodeJSONObjectFlag("data", f.data)
				if err != nil {
					return err
				}
				message["data"] = d
			}
			if f.routing != "" {
				r, err := decodeJSONObjectFlag("routing", f.routing)
				if err != nil {
					return err
				}
				message["routing"] = r
			}
			if f.brandID != "" {
				message["brand_id"] = f.brandID
			}
			payload := map[string]any{"message": message}
			out, err := s.call(cmd.Context(), key, http.MethodPost, "/send", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(out)
		},
	}
	pf := cmd.Flags()
	pf.StringVar(&f.userID, "user-id", "", "recipient user id")
	pf.StringVar(&f.email, "email", "", "recipient email address")
	pf.StringVar(&f.phone, "phone", "", "recipient phone number")
	pf.StringVar(&f.listID, "list-id", "", "recipient list id")
	pf.StringVar(&f.audienceID, "audience-id", "", "recipient audience id")
	pf.StringVar(&f.template, "template", "", "notification template id (XOR --title/--body)")
	pf.StringVar(&f.title, "title", "", "inline content title (with --body)")
	pf.StringVar(&f.body, "body", "", "inline content body (with --title)")
	pf.StringVar(&f.data, "data", "", "template variables as a JSON object")
	pf.StringVar(&f.routing, "routing", "", "delivery routing as a JSON object")
	pf.StringVar(&f.brandID, "brand-id", "", "brand id for rendering")
	return cmd
}

// recipient resolves the single `to` object, erroring if zero or more than one
// selector is set.
func (f sendFlags) recipient() (map[string]any, error) {
	selectors := []struct {
		field string
		value string
	}{
		{"user_id", f.userID},
		{"email", f.email},
		{"phone_number", f.phone},
		{"list_id", f.listID},
		{"audience_id", f.audienceID},
	}
	var chosen map[string]any
	count := 0
	for _, sel := range selectors {
		if sel.value != "" {
			chosen = map[string]any{sel.field: sel.value}
			count++
		}
	}
	if count == 0 {
		return nil, &usageError{msg: "courier send: one recipient is required (--user-id / --email / --phone / --list-id / --audience-id)"}
	}
	if count > 1 {
		return nil, &usageError{msg: "courier send: only one recipient selector may be set"}
	}
	return chosen, nil
}

// content resolves the message content: a template id XOR an inline title+body.
func (f sendFlags) content() (map[string]any, error) {
	hasTemplate := f.template != ""
	hasInline := f.title != "" || f.body != ""
	switch {
	case hasTemplate && hasInline:
		return nil, &usageError{msg: "courier send: use --template or --title/--body, not both"}
	case hasTemplate:
		return map[string]any{"template": f.template}, nil
	case hasInline:
		if f.title == "" || f.body == "" {
			return nil, &usageError{msg: "courier send: inline content requires both --title and --body"}
		}
		return map[string]any{"content": map[string]any{"title": f.title, "body": f.body}}, nil
	default:
		return nil, &usageError{msg: "courier send: content is required (--template or --title/--body)"}
	}
}
