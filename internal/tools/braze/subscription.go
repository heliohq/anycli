package braze

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newSubscriptionCmd builds the `subscription` resource group: status-get (GET,
// read a user's subscription-group state) and status-set (POST, subscribe /
// unsubscribe — permission-gated).
func (s *Service) newSubscriptionCmd(c *client) *cobra.Command {
	group := newGroupCmd("subscription", "Subscription-group status read and update")
	group.AddCommand(
		s.newSubscriptionStatusGetCmd(c),
		s.newSubscriptionStatusSetCmd(c),
	)
	return group
}

// newSubscriptionStatusGetCmd is `subscription status-get` (GET
// /subscription/user/status): a user's subscription-group states.
func (s *Service) newSubscriptionStatusGetCmd(c *client) *cobra.Command {
	var externalID, email, phone string
	var limit, offset int
	cmd := &cobra.Command{
		Use:   "status-get",
		Short: "Get a user's subscription-group states",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().StringVar(&externalID, "external-id", "", "external user id (required unless --email/--phone given)")
	cmd.Flags().StringVar(&email, "email", "", "email address (alternative identifier)")
	cmd.Flags().StringVar(&phone, "phone", "", "E.164 phone number (alternative identifier)")
	cmd.Flags().IntVar(&limit, "limit", 0, "max subscription groups to return")
	cmd.Flags().IntVar(&offset, "offset", 0, "number of subscription groups to skip")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if externalID == "" && email == "" && phone == "" {
			return &usageError{msg: "subscription status-get requires one of --external-id, --email, --phone"}
		}
		q := url.Values{}
		if externalID != "" {
			q.Set("external_id", externalID)
		}
		if email != "" {
			q.Set("email", email)
		}
		if phone != "" {
			q.Set("phone", phone)
		}
		if cmd.Flags().Changed("limit") {
			q.Set("limit", strconv.Itoa(limit))
		}
		if cmd.Flags().Changed("offset") {
			q.Set("offset", strconv.Itoa(offset))
		}
		body, err := c.get(cmd.Context(), "/subscription/user/status", q)
		if err != nil {
			return err
		}
		return c.emit(body)
	}
	return cmd
}

// newSubscriptionStatusSetCmd is `subscription status-set` (POST
// /subscription/status/set): set a user's state in a subscription group.
// Permission-gated.
func (s *Service) newSubscriptionStatusSetCmd(c *client) *cobra.Command {
	var groupID, state string
	var externalIDs, emails []string
	cmd := &cobra.Command{
		Use:   "status-set",
		Short: "Set a user's subscription-group state (permission-gated)",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().StringVar(&groupID, "subscription-group-id", "", "subscription group id (required)")
	cmd.Flags().StringVar(&state, "state", "", "subscribed|unsubscribed (required)")
	cmd.Flags().StringArrayVar(&externalIDs, "external-id", nil, "external user id (repeatable)")
	cmd.Flags().StringArrayVar(&emails, "email", nil, "email address (repeatable)")
	_ = cmd.MarkFlagRequired("subscription-group-id")
	_ = cmd.MarkFlagRequired("state")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if state != "subscribed" && state != "unsubscribed" {
			return &usageError{msg: "--state must be subscribed or unsubscribed"}
		}
		if len(externalIDs) == 0 && len(emails) == 0 {
			return &usageError{msg: "subscription status-set requires at least one of --external-id, --email"}
		}
		payload := map[string]any{
			"subscription_group_id": groupID,
			"subscription_state":    state,
		}
		if len(externalIDs) > 0 {
			payload["external_id"] = externalIDs
		}
		if len(emails) > 0 {
			payload["email"] = emails
		}
		body, err := c.post(cmd.Context(), "/subscription/status/set", payload)
		if err != nil {
			return err
		}
		return c.emit(body)
	}
	return cmd
}
