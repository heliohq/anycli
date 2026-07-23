package braze

import (
	"github.com/spf13/cobra"
)

// newUsersCmd builds the `users` resource group: export (profile lookup, POST)
// and track (identify / attribute / event, POST — permission-gated).
func (s *Service) newUsersCmd(c *client) *cobra.Command {
	group := newGroupCmd("users", "User-profile lookup and identify/track")
	group.AddCommand(
		s.newUsersExportCmd(c),
		s.newUsersTrackCmd(c),
	)
	return group
}

// newUsersExportCmd is `users export` (POST /users/export/ids): look up user
// profiles by identifier. Braze exports by POST, not GET. At least one
// identifier is required.
func (s *Service) newUsersExportCmd(c *client) *cobra.Command {
	var externalIDs, fields []string
	var email, brazeID string
	cmd := &cobra.Command{
		Use:         "export",
		Short:       "Look up user profiles by identifier (POST /users/export/ids)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	// Braze's /users/export/ids contract: external_ids is an array (up to 50),
	// but email_address and braze_id are single strings ("only one email_address
	// or device_id can be included per request"). Model them as single-value flags.
	cmd.Flags().StringArrayVar(&externalIDs, "external-id", nil, "external user id (repeatable)")
	cmd.Flags().StringVar(&email, "email", "", "email address (single)")
	cmd.Flags().StringVar(&brazeID, "braze-id", "", "Braze internal user id (single)")
	cmd.Flags().StringArrayVar(&fields, "fields", nil, "profile field to export (repeatable; omit for defaults)")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		payload := map[string]any{}
		if len(externalIDs) > 0 {
			payload["external_ids"] = externalIDs
		}
		if email != "" {
			payload["email_address"] = email
		}
		if brazeID != "" {
			payload["braze_id"] = brazeID
		}
		if len(fields) > 0 {
			payload["fields_to_export"] = fields
		}
		if len(externalIDs) == 0 && email == "" && brazeID == "" {
			return &usageError{msg: "users export requires at least one of --external-id, --email, --braze-id"}
		}
		body, err := c.post(cmd.Context(), "/users/export/ids", payload)
		if err != nil {
			return err
		}
		return c.emit(body)
	}
	return cmd
}

// newUsersTrackCmd is `users track` (POST /users/track): identify users and
// record attributes / events / purchases. The large, versioned Braze payloads
// are passed through as raw JSON arrays; the tool only assembles the envelope.
// Permission-gated by the REST key's scope; acts on live customer data.
func (s *Service) newUsersTrackCmd(c *client) *cobra.Command {
	var attributesFlag, eventsFlag, purchasesFlag string
	cmd := &cobra.Command{
		Use:         "track",
		Short:       "Identify users and record attributes/events/purchases (permission-gated)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
	}
	cmd.Flags().StringVar(&attributesFlag, "attributes", "", "raw JSON array of attribute objects")
	cmd.Flags().StringVar(&eventsFlag, "events", "", "raw JSON array of custom-event objects")
	cmd.Flags().StringVar(&purchasesFlag, "purchases", "", "raw JSON array of purchase objects")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		payload := map[string]any{}
		for name, raw := range map[string]string{
			"attributes": attributesFlag,
			"events":     eventsFlag,
			"purchases":  purchasesFlag,
		} {
			if raw == "" {
				continue
			}
			v, err := decodeJSONFlag(name, raw)
			if err != nil {
				return err
			}
			payload[name] = v
		}
		if len(payload) == 0 {
			return &usageError{msg: "users track requires at least one of --attributes, --events, --purchases"}
		}
		body, err := c.post(cmd.Context(), "/users/track", payload)
		if err != nil {
			return err
		}
		return c.emit(body)
	}
	return cmd
}
