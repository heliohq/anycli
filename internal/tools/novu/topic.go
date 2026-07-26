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

// The topic Longs, grouped above the constructors that use them because every
// leaf in this group is built by the shared leafCmd helper.
const (
	longTopicList = "A topic is addressed by its KEY, a string the caller chose, and that key\n" +
		"is what `event trigger --to-json` needs — this list is how to recover one\n" +
		"when only a human name is known. The --key and --name filters are exact\n" +
		"match. Paging is cursor-based through --after and --before."

	longTopicCreate = "--key is the permanent handle triggers target and cannot be renamed later;\n" +
		"--name is a human label only. Creating a topic does not populate it — a\n" +
		"new topic has no subscribers, and a send to it reaches nobody until\n" +
		"`topic add-subscribers` has run."

	longTopicGet = "--key is the topic key, not an id. It returns the topic record only: no\n" +
		"command in this tool enumerates a topic's members, so \"who is in this\n" +
		"audience\" cannot be answered here."

	longTopicAddSubscribers = "--subscriber-ids is comma-separated and every id must already exist as a\n" +
		"subscriber — this does not create anyone. Adding someone already in the\n" +
		"topic is not an error. Membership applies to the NEXT trigger; a send\n" +
		"already accepted for this topic is unaffected."

	longTopicRemoveSubscribers = "--subscriber-ids is comma-separated and removes membership only — the\n" +
		"subscribers themselves, their preferences and their other topics are\n" +
		"untouched. It does not stop a trigger Novu has already accepted for the\n" +
		"topic, since recipients are resolved as that trigger is processed."
)

func (s *Service) newTopicListCmd(c *client) *cobra.Command {
	var key, name, after, before string
	var limit int
	cmd := leafCmd("list", "List topics", longTopicList, readOnly, func(cmd *cobra.Command, _ []string) error {
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
	cmd := leafCmd("create", "Create a topic", longTopicCreate, writeAction, func(cmd *cobra.Command, _ []string) error {
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
	cmd := leafCmd("get", "Get one topic by key", longTopicGet, readOnly, func(cmd *cobra.Command, _ []string) error {
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
	cmd := leafCmd("add-subscribers", "Add subscribers to a topic", longTopicAddSubscribers, writeAction, func(cmd *cobra.Command, _ []string) error {
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
	cmd := leafCmd("remove-subscribers", "Remove subscribers from a topic", longTopicRemoveSubscribers, writeAction, func(cmd *cobra.Command, _ []string) error {
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
