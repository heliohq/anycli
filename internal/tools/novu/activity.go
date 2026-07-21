package novu

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newActivityCmd is the `activity` group over /v1/notifications: the activity
// feed for debugging a triggered run.
func (s *Service) newActivityCmd(c *client) *cobra.Command {
	group := newGroupCmd("activity", "Inspect the activity feed")
	group.AddCommand(
		s.newActivityListCmd(c),
		s.newActivityGetCmd(c),
	)
	return group
}

func (s *Service) newActivityListCmd(c *client) *cobra.Command {
	var channels, templates, subscriberIDs, search, transactionID, topicKey string
	var page, limit int
	cmd := leafCmd("list", "List activity-feed notifications", func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		// channels / templates / subscriberIds / emails are repeatable array
		// params; send one entry per comma-separated value.
		addRepeated(q, "channels", channels)
		addRepeated(q, "templates", templates)
		addRepeated(q, "subscriberIds", subscriberIDs)
		addQueryString(q, "search", search)
		addQueryString(q, "transactionId", transactionID)
		addQueryString(q, "topicKey", topicKey)
		addQueryInt(q, "page", page)
		addQueryInt(q, "limit", limit)
		out, err := c.call(cmd.Context(), http.MethodGet, "/v1/notifications", q, nil)
		if err != nil {
			return err
		}
		return s.emit(out)
	})
	f := cmd.Flags()
	f.StringVar(&channels, "channels", "", "comma-separated channels: in_app|email|sms|chat|push")
	f.StringVar(&templates, "templates", "", "comma-separated workflow/template ids")
	f.StringVar(&subscriberIDs, "subscriber-ids", "", "comma-separated subscriberIds")
	f.StringVar(&search, "search", "", "free-text search")
	f.StringVar(&transactionID, "transaction-id", "", "filter by transactionId")
	f.StringVar(&topicKey, "topic-key", "", "filter by topic key")
	f.IntVar(&page, "page", 0, "page number (0-based)")
	f.IntVar(&limit, "limit", 0, "max results per page")
	return cmd
}

func (s *Service) newActivityGetCmd(c *client) *cobra.Command {
	var id string
	cmd := leafCmd("get", "Get one activity-feed notification by id", func(cmd *cobra.Command, _ []string) error {
		if err := requireFlag("notification-id", id); err != nil {
			return err
		}
		out, err := c.call(cmd.Context(), http.MethodGet, "/v1/notifications/"+pathEscape(id), nil, nil)
		if err != nil {
			return err
		}
		return s.emit(out)
	})
	cmd.Flags().StringVar(&id, "notification-id", "", "notification id (required)")
	return cmd
}

// addRepeated adds one query entry per comma-separated value, matching Novu's
// repeated array-param convention (?channels=email&channels=sms).
func addRepeated(q url.Values, key, raw string) {
	for _, v := range splitCSV(raw) {
		q.Add(key, v)
	}
}
