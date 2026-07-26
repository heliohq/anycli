package novu

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newSubscriberCmd is the `subscriber` group over /v2/subscribers: recipients
// and their channel identifiers (email/phone) and notification preferences.
func (s *Service) newSubscriberCmd(c *client) *cobra.Command {
	group := newGroupCmd("subscriber", "Manage recipients (subscribers)")
	group.AddCommand(
		s.newSubscriberListCmd(c),
		s.newSubscriberGetCmd(c),
		s.newSubscriberCreateCmd(c),
		s.newSubscriberUpdateCmd(c),
		s.newSubscriberDeleteCmd(c),
		s.newSubscriberPreferencesCmd(c),
		s.newSubscriberSetPreferencesCmd(c),
	)
	return group
}

// The subscriber Longs, grouped above the constructors that use them because
// every leaf in this group is built by the shared leafCmd helper.
const (
	longSubscriberList = "The filters are exact-match — --email, --phone, --subscriber-id and --name\n" +
		"are not a fuzzy search, so a partial address finds nothing. Paging is\n" +
		"cursor-based: --after and --before take ids from a previous page, not\n" +
		"offsets, and --limit 0 leaves Novu's own page size."

	longSubscriberGet = "--subscriber-id is the CALLER's id for the person — the one supplied at\n" +
		"create time, usually the application's own user id — not a Novu-generated\n" +
		"key. The response's channel identifiers (email, phone, push tokens) are\n" +
		"what decide which of a workflow's steps can reach them at all."

	longSubscriberCreate = "--subscriber-id is required and becomes the stable handle every trigger\n" +
		"addresses. The channel identifiers matter more than the name fields: a\n" +
		"workflow's email step has nothing to deliver to when --email was never\n" +
		"set, and the trigger still reports success. --data takes a JSON object of\n" +
		"custom attributes that workflow templates and step conditions can read."

	longSubscriberUpdate = "A PATCH: only flags actually passed are sent, so omitted fields keep their\n" +
		"values. --subscriber-id selects the record and is not itself editable\n" +
		"here. --data travels as one whole object, so it must contain every custom\n" +
		"attribute that should survive the write."

	longSubscriberDelete = "Removes the subscriber along with their channel identifiers, preferences\n" +
		"and topic memberships. It is not durable suppression: a later trigger\n" +
		"whose --to-json carries a full subscriber object recreates them. To stop\n" +
		"sending to someone while keeping their history, use\n" +
		"`subscriber set-preferences` instead."

	longSubscriberPreferences = "Per-workflow, per-channel opt-in state for one subscriber, which is what\n" +
		"ultimately decides whether a triggered workflow reaches them. A perfectly\n" +
		"valid subscriber can receive nothing because a channel is switched off\n" +
		"here, so this is worth reading before blaming the workflow or the\n" +
		"provider."

	longSubscriberSetPreferences = "--preferences is the raw JSON preferences object, passed through with only\n" +
		"a syntax check, so read the current shape with `subscriber preferences`\n" +
		"before composing one. This is the durable way to record an opt-out: it\n" +
		"suppresses the channel while keeping the subscriber and their delivery\n" +
		"history, unlike `subscriber delete`."
)

func (s *Service) newSubscriberListCmd(c *client) *cobra.Command {
	var email, name, phone, subscriberID, after, before, orderBy, orderDirection string
	var limit int
	cmd := leafCmd("list", "List / search subscribers", longSubscriberList, readOnly, func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		addQueryString(q, "email", email)
		addQueryString(q, "name", name)
		addQueryString(q, "phone", phone)
		addQueryString(q, "subscriberId", subscriberID)
		addQueryString(q, "after", after)
		addQueryString(q, "before", before)
		addQueryString(q, "orderBy", orderBy)
		addQueryString(q, "orderDirection", orderDirection)
		addQueryInt(q, "limit", limit)
		out, err := c.call(cmd.Context(), http.MethodGet, "/v2/subscribers", q, nil)
		if err != nil {
			return err
		}
		return s.emit(out)
	})
	f := cmd.Flags()
	f.StringVar(&email, "email", "", "filter by email")
	f.StringVar(&name, "name", "", "filter by name")
	f.StringVar(&phone, "phone", "", "filter by phone")
	f.StringVar(&subscriberID, "subscriber-id", "", "filter by subscriberId")
	f.StringVar(&after, "after", "", "cursor: page after this id")
	f.StringVar(&before, "before", "", "cursor: page before this id")
	f.StringVar(&orderBy, "order-by", "", "field to order by")
	f.StringVar(&orderDirection, "order-direction", "", "ASC or DESC")
	f.IntVar(&limit, "limit", 0, "max results per page")
	return cmd
}

func (s *Service) newSubscriberGetCmd(c *client) *cobra.Command {
	var id string
	cmd := leafCmd("get", "Get one subscriber by id", longSubscriberGet, readOnly, func(cmd *cobra.Command, _ []string) error {
		if err := requireFlag("subscriber-id", id); err != nil {
			return err
		}
		out, err := c.call(cmd.Context(), http.MethodGet, "/v2/subscribers/"+pathEscape(id), nil, nil)
		if err != nil {
			return err
		}
		return s.emit(out)
	})
	cmd.Flags().StringVar(&id, "subscriber-id", "", "subscriberId (required)")
	return cmd
}

func (s *Service) newSubscriberCreateCmd(c *client) *cobra.Command {
	var id, email, firstName, lastName, phone, avatar, locale, timezone, data string
	cmd := leafCmd("create", "Create a subscriber", longSubscriberCreate, writeAction, func(cmd *cobra.Command, _ []string) error {
		if err := requireFlag("subscriber-id", id); err != nil {
			return err
		}
		body := map[string]any{"subscriberId": id}
		applySubscriberFields(body, email, firstName, lastName, phone, avatar, locale, timezone)
		if err := putJSON(body, "data", data); err != nil {
			return err
		}
		out, err := c.call(cmd.Context(), http.MethodPost, "/v2/subscribers", nil, body)
		if err != nil {
			return err
		}
		return s.emit(out)
	})
	registerSubscriberFieldFlags(cmd, &id, &email, &firstName, &lastName, &phone, &avatar, &locale, &timezone, &data, true)
	return cmd
}

func (s *Service) newSubscriberUpdateCmd(c *client) *cobra.Command {
	var id, email, firstName, lastName, phone, avatar, locale, timezone, data string
	cmd := leafCmd("update", "Update a subscriber (PATCH)", longSubscriberUpdate, writeAction, func(cmd *cobra.Command, _ []string) error {
		if err := requireFlag("subscriber-id", id); err != nil {
			return err
		}
		body := map[string]any{}
		applySubscriberFields(body, email, firstName, lastName, phone, avatar, locale, timezone)
		if err := putJSON(body, "data", data); err != nil {
			return err
		}
		out, err := c.call(cmd.Context(), http.MethodPatch, "/v2/subscribers/"+pathEscape(id), nil, body)
		if err != nil {
			return err
		}
		return s.emit(out)
	})
	registerSubscriberFieldFlags(cmd, &id, &email, &firstName, &lastName, &phone, &avatar, &locale, &timezone, &data, false)
	return cmd
}

func (s *Service) newSubscriberDeleteCmd(c *client) *cobra.Command {
	var id string
	cmd := leafCmd("delete", "Delete a subscriber", longSubscriberDelete, writeAction, func(cmd *cobra.Command, _ []string) error {
		if err := requireFlag("subscriber-id", id); err != nil {
			return err
		}
		out, err := c.call(cmd.Context(), http.MethodDelete, "/v2/subscribers/"+pathEscape(id), nil, nil)
		if err != nil {
			return err
		}
		return s.emit(out)
	})
	cmd.Flags().StringVar(&id, "subscriber-id", "", "subscriberId (required)")
	return cmd
}

func (s *Service) newSubscriberPreferencesCmd(c *client) *cobra.Command {
	var id string
	cmd := leafCmd("preferences", "Get a subscriber's channel preferences", longSubscriberPreferences, readOnly, func(cmd *cobra.Command, _ []string) error {
		if err := requireFlag("subscriber-id", id); err != nil {
			return err
		}
		out, err := c.call(cmd.Context(), http.MethodGet, "/v2/subscribers/"+pathEscape(id)+"/preferences", nil, nil)
		if err != nil {
			return err
		}
		return s.emit(out)
	})
	cmd.Flags().StringVar(&id, "subscriber-id", "", "subscriberId (required)")
	return cmd
}

func (s *Service) newSubscriberSetPreferencesCmd(c *client) *cobra.Command {
	var id, preferences string
	cmd := leafCmd("set-preferences", "Update a subscriber's channel preferences (PATCH)", longSubscriberSetPreferences, writeAction, func(cmd *cobra.Command, _ []string) error {
		if err := requireFlag("subscriber-id", id); err != nil {
			return err
		}
		decoded, err := decodeJSONFlag("preferences", preferences)
		if err != nil {
			return err
		}
		if decoded == nil {
			return &usageError{msg: "novu: --preferences is required (a JSON preferences object)"}
		}
		out, err := c.call(cmd.Context(), http.MethodPatch, "/v2/subscribers/"+pathEscape(id)+"/preferences", nil, decoded)
		if err != nil {
			return err
		}
		return s.emit(out)
	})
	f := cmd.Flags()
	f.StringVar(&id, "subscriber-id", "", "subscriberId (required)")
	f.StringVar(&preferences, "preferences", "", "preferences payload as JSON (required)")
	return cmd
}

// applySubscriberFields writes the optional subscriber profile fields into a
// request body, omitting empties.
func applySubscriberFields(body map[string]any, email, firstName, lastName, phone, avatar, locale, timezone string) {
	setIfNonEmpty(body, "email", email)
	setIfNonEmpty(body, "firstName", firstName)
	setIfNonEmpty(body, "lastName", lastName)
	setIfNonEmpty(body, "phone", phone)
	setIfNonEmpty(body, "avatar", avatar)
	setIfNonEmpty(body, "locale", locale)
	setIfNonEmpty(body, "timezone", timezone)
}

// registerSubscriberFieldFlags wires the shared profile flags onto create/update.
func registerSubscriberFieldFlags(cmd *cobra.Command, id, email, firstName, lastName, phone, avatar, locale, timezone, data *string, _ bool) {
	f := cmd.Flags()
	f.StringVar(id, "subscriber-id", "", "subscriberId (required)")
	f.StringVar(email, "email", "", "email channel identifier")
	f.StringVar(firstName, "first-name", "", "first name")
	f.StringVar(lastName, "last-name", "", "last name")
	f.StringVar(phone, "phone", "", "phone channel identifier")
	f.StringVar(avatar, "avatar", "", "avatar URL")
	f.StringVar(locale, "locale", "", "locale (e.g. en_US)")
	f.StringVar(timezone, "timezone", "", "IANA timezone")
	f.StringVar(data, "data", "", "custom data as a JSON object")
}
