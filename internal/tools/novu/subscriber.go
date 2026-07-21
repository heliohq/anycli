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

func (s *Service) newSubscriberListCmd(c *client) *cobra.Command {
	var email, name, phone, subscriberID, after, before, orderBy, orderDirection string
	var limit int
	cmd := leafCmd("list", "List / search subscribers", func(cmd *cobra.Command, _ []string) error {
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
	cmd := leafCmd("get", "Get one subscriber by id", func(cmd *cobra.Command, _ []string) error {
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
	cmd := leafCmd("create", "Create a subscriber", func(cmd *cobra.Command, _ []string) error {
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
	cmd := leafCmd("update", "Update a subscriber (PATCH)", func(cmd *cobra.Command, _ []string) error {
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
	cmd := leafCmd("delete", "Delete a subscriber", func(cmd *cobra.Command, _ []string) error {
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
	cmd := leafCmd("preferences", "Get a subscriber's channel preferences", func(cmd *cobra.Command, _ []string) error {
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
	cmd := leafCmd("set-preferences", "Update a subscriber's channel preferences (PATCH)", func(cmd *cobra.Command, _ []string) error {
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
