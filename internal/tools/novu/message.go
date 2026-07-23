package novu

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newMessageCmd is the `message` group over /v1/messages: delivery inspection,
// filterable by channel / subscriber / transactionId.
func (s *Service) newMessageCmd(c *client) *cobra.Command {
	group := newGroupCmd("message", "Inspect delivered messages")
	group.AddCommand(
		s.newMessageListCmd(c),
		s.newMessageDeleteCmd(c),
	)
	return group
}

func (s *Service) newMessageListCmd(c *client) *cobra.Command {
	var channel, subscriberID, transactionID string
	var page, limit int
	cmd := leafCmd("list", "List messages", readOnly, func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		addQueryString(q, "channel", channel)
		addQueryString(q, "subscriberId", subscriberID)
		addQueryString(q, "transactionId", transactionID)
		addQueryInt(q, "limit", limit)
		// page is 0-based; send it explicitly when the caller sets a positive value.
		addQueryInt(q, "page", page)
		out, err := c.call(cmd.Context(), http.MethodGet, "/v1/messages", q, nil)
		if err != nil {
			return err
		}
		return s.emit(out)
	})
	f := cmd.Flags()
	f.StringVar(&channel, "channel", "", "channel: in_app|email|sms|chat|push")
	f.StringVar(&subscriberID, "subscriber-id", "", "filter by subscriberId")
	f.StringVar(&transactionID, "transaction-id", "", "filter by transactionId")
	f.IntVar(&page, "page", 0, "page number (0-based)")
	f.IntVar(&limit, "limit", 0, "max results per page")
	return cmd
}

func (s *Service) newMessageDeleteCmd(c *client) *cobra.Command {
	var id string
	cmd := leafCmd("delete", "Delete a message by id", writeAction, func(cmd *cobra.Command, _ []string) error {
		if err := requireFlag("message-id", id); err != nil {
			return err
		}
		out, err := c.call(cmd.Context(), http.MethodDelete, "/v1/messages/"+pathEscape(id), nil, nil)
		if err != nil {
			return err
		}
		return s.emit(out)
	})
	cmd.Flags().StringVar(&id, "message-id", "", "message id (required)")
	return cmd
}
