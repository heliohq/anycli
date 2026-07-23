package novu

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newTopicCmd is the `topic` group over /v2/topics: audience grouping for
// broadcast-to-segment sends. Subscribers join/leave a topic via its
// subscriptions collection.
func (s *Service) newTopicCmd(c *client) *cobra.Command {
	group := newGroupCmd("topic", "Manage audiences (topics)")
	group.AddCommand(
		s.newTopicListCmd(c),
		s.newTopicCreateCmd(c),
		s.newTopicGetCmd(c),
		s.newTopicAddSubscribersCmd(c),
		s.newTopicRemoveSubscribersCmd(c),
	)
	return group
}

func (s *Service) newTopicListCmd(c *client) *cobra.Command {
	var key, name, after, before string
	var limit int
	cmd := leafCmd("list", "List topics", readOnly, func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		addQueryString(q, "key", key)
		addQueryString(q, "name", name)
		addQueryString(q, "after", after)
		addQueryString(q, "before", before)
		addQueryInt(q, "limit", limit)
		out, err := c.call(cmd.Context(), http.MethodGet, "/v2/topics", q, nil)
		if err != nil {
			return err
		}
		return s.emit(out)
	})
	f := cmd.Flags()
	f.StringVar(&key, "key", "", "filter by topic key")
	f.StringVar(&name, "name", "", "filter by name")
	f.StringVar(&after, "after", "", "cursor: page after this id")
	f.StringVar(&before, "before", "", "cursor: page before this id")
	f.IntVar(&limit, "limit", 0, "max results per page")
	return cmd
}

func (s *Service) newTopicCreateCmd(c *client) *cobra.Command {
	var key, name string
	cmd := leafCmd("create", "Create a topic", writeAction, func(cmd *cobra.Command, _ []string) error {
		if err := requireFlag("key", key); err != nil {
			return err
		}
		body := map[string]any{"key": key}
		setIfNonEmpty(body, "name", name)
		out, err := c.call(cmd.Context(), http.MethodPost, "/v2/topics", nil, body)
		if err != nil {
			return err
		}
		return s.emit(out)
	})
	f := cmd.Flags()
	f.StringVar(&key, "key", "", "topic key (required)")
	f.StringVar(&name, "name", "", "human-readable topic name")
	return cmd
}

func (s *Service) newTopicGetCmd(c *client) *cobra.Command {
	var key string
	cmd := leafCmd("get", "Get one topic by key", readOnly, func(cmd *cobra.Command, _ []string) error {
		if err := requireFlag("key", key); err != nil {
			return err
		}
		out, err := c.call(cmd.Context(), http.MethodGet, "/v2/topics/"+pathEscape(key), nil, nil)
		if err != nil {
			return err
		}
		return s.emit(out)
	})
	cmd.Flags().StringVar(&key, "key", "", "topic key (required)")
	return cmd
}

func (s *Service) newTopicAddSubscribersCmd(c *client) *cobra.Command {
	var key, subscriberIDs string
	cmd := leafCmd("add-subscribers", "Add subscribers to a topic", writeAction, func(cmd *cobra.Command, _ []string) error {
		if err := requireFlag("key", key); err != nil {
			return err
		}
		ids := splitCSV(subscriberIDs)
		if len(ids) == 0 {
			return &usageError{msg: "novu: --subscriber-ids is required (comma-separated)"}
		}
		body := map[string]any{"subscriberIds": ids}
		out, err := c.call(cmd.Context(), http.MethodPost, "/v2/topics/"+pathEscape(key)+"/subscriptions", nil, body)
		if err != nil {
			return err
		}
		return s.emit(out)
	})
	f := cmd.Flags()
	f.StringVar(&key, "key", "", "topic key (required)")
	f.StringVar(&subscriberIDs, "subscriber-ids", "", "comma-separated subscriberIds (required)")
	return cmd
}

func (s *Service) newTopicRemoveSubscribersCmd(c *client) *cobra.Command {
	var key, subscriberIDs string
	cmd := leafCmd("remove-subscribers", "Remove subscribers from a topic", writeAction, func(cmd *cobra.Command, _ []string) error {
		if err := requireFlag("key", key); err != nil {
			return err
		}
		ids := splitCSV(subscriberIDs)
		if len(ids) == 0 {
			return &usageError{msg: "novu: --subscriber-ids is required (comma-separated)"}
		}
		body := map[string]any{"subscriberIds": ids}
		out, err := c.call(cmd.Context(), http.MethodDelete, "/v2/topics/"+pathEscape(key)+"/subscriptions", nil, body)
		if err != nil {
			return err
		}
		return s.emit(out)
	})
	f := cmd.Flags()
	f.StringVar(&key, "key", "", "topic key (required)")
	f.StringVar(&subscriberIDs, "subscriber-ids", "", "comma-separated subscriberIds (required)")
	return cmd
}
