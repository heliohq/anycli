package loops

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newContactCmd groups contact CRM operations.
func (s *Service) newContactCmd(key string) *cobra.Command {
	cmd := newGroup("contact", "Contacts (create, update, find, delete, suppression)")
	cmd.AddCommand(
		s.newContactCreateCmd(key),
		s.newContactUpdateCmd(key),
		s.newContactFindCmd(key),
		s.newContactDeleteCmd(key),
		s.newContactSuppressionCmd(key),
	)
	return cmd
}

// contactFlags holds the first-class ContactFields plus the custom-property
// escape hatches, wired as named convenience flags so the common attributes
// (firstName/lastName/…) are intention-revealing rather than buried in raw JSON.
type contactFlags struct {
	email       string
	firstName   string
	lastName    string
	source      string
	userGroup   string
	userID      string
	subscribed  bool
	mailingList []string
	property    []string
	propsJSON   string
}

// register wires the shared ContactFields flags onto a command. The email /
// user-id flags are registered by the caller (create marks email required;
// update marks one-of-email-or-userId), so they are not registered here.
func (f *contactFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.firstName, "first-name", "", "contact first name")
	cmd.Flags().StringVar(&f.lastName, "last-name", "", "contact last name")
	cmd.Flags().StringVar(&f.source, "source", "", "custom source value (replaces the default \"API\")")
	cmd.Flags().StringVar(&f.userGroup, "user-group", "", "user group for segmentation")
	cmd.Flags().BoolVar(&f.subscribed, "subscribed", false, "subscription state (only sent when the flag is set)")
	cmd.Flags().StringArrayVar(&f.mailingList, "mailing-list", nil, "mailing-list subscription id=true|false (repeatable)")
	cmd.Flags().StringArrayVar(&f.property, "property", nil, "custom contact property key=value, typed-coerced (repeatable)")
	cmd.Flags().StringVar(&f.propsJSON, "properties-json", "", "custom properties as a raw JSON object (merged into the body)")
}

// body builds the request body from the resolved flags. subscribed is included
// only when the flag was explicitly set (per Loops guidance — leaving it out
// avoids accidental (un)subscribes on update). Custom properties from
// --property and --properties-json are merged at the top level; a duplicate key
// in --property wins over --properties-json.
func (f *contactFlags) body(cmd *cobra.Command) (map[string]any, error) {
	body := map[string]any{}
	if f.propsJSON != "" {
		props, err := decodeJSONObject("properties-json", f.propsJSON)
		if err != nil {
			return nil, err
		}
		for k, v := range props {
			body[k] = v
		}
	}
	props, err := parseKeyValues("property", f.property)
	if err != nil {
		return nil, err
	}
	for k, v := range props {
		body[k] = v
	}
	mailing, err := parseMailingLists(f.mailingList)
	if err != nil {
		return nil, err
	}
	if mailing != nil {
		body["mailingLists"] = mailing
	}
	if f.email != "" {
		body["email"] = f.email
	}
	if f.userID != "" {
		body["userId"] = f.userID
	}
	setIfNotEmpty(body, "firstName", f.firstName)
	setIfNotEmpty(body, "lastName", f.lastName)
	setIfNotEmpty(body, "source", f.source)
	setIfNotEmpty(body, "userGroup", f.userGroup)
	if cmd.Flags().Changed("subscribed") {
		body["subscribed"] = f.subscribed
	}
	return body, nil
}

func setIfNotEmpty(body map[string]any, field, value string) {
	if value != "" {
		body[field] = value
	}
}

func (s *Service) newContactCreateCmd(key string) *cobra.Command {
	f := &contactFlags{}
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a contact (POST /v1/contacts/create)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := f.body(cmd)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), key, http.MethodPost, "/v1/contacts/create", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&f.email, "email", "", "contact email address (required)")
	cmd.Flags().StringVar(&f.userID, "user-id", "", "external unique user id")
	f.register(cmd)
	_ = cmd.MarkFlagRequired("email")
	return cmd
}

func (s *Service) newContactUpdateCmd(key string) *cobra.Command {
	f := &contactFlags{}
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update or upsert a contact (PUT /v1/contacts/update)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := f.body(cmd)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), key, http.MethodPut, "/v1/contacts/update", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	// update requires at least one identifier (anyOf email/userId); both are
	// allowed (e.g. attach a userId to an email-identified contact).
	cmd.Flags().StringVar(&f.email, "email", "", "contact email address (required if --user-id is absent)")
	cmd.Flags().StringVar(&f.userID, "user-id", "", "external unique user id (required if --email is absent)")
	f.register(cmd)
	cmd.MarkFlagsOneRequired("email", "user-id")
	return cmd
}

func (s *Service) newContactFindCmd(key string) *cobra.Command {
	var email, userID string
	cmd := &cobra.Command{
		Use:   "find",
		Short: "Find a contact by email or userId (GET /v1/contacts/find)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := singleIdentifierQuery(email, userID)
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/v1/contacts/find", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerExactlyOneIdentifier(cmd, &email, &userID)
	return cmd
}

func (s *Service) newContactDeleteCmd(key string) *cobra.Command {
	var email, userID string
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a contact by email or userId (POST /v1/contacts/delete)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// The OpenAPI schema lists both email and userId as required, but the
			// endpoint docs and live API reject the both-provided case (HTTP 400
			// "email and userId are both provided"). cobra's mutually-exclusive
			// group enforces exactly one client-side, and we forward only that one.
			body := map[string]any{}
			if email != "" {
				body["email"] = email
			} else {
				body["userId"] = userID
			}
			resp, err := s.call(cmd.Context(), key, http.MethodPost, "/v1/contacts/delete", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerExactlyOneIdentifier(cmd, &email, &userID)
	return cmd
}

func (s *Service) newContactSuppressionCmd(key string) *cobra.Command {
	cmd := newGroup("suppression", "Contact suppression status")
	cmd.AddCommand(
		s.newContactSuppressionGetCmd(key),
		s.newContactSuppressionRemoveCmd(key),
	)
	return cmd
}

func (s *Service) newContactSuppressionGetCmd(key string) *cobra.Command {
	var email, userID string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get suppression status by email or userId (GET /v1/contacts/suppression)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := singleIdentifierQuery(email, userID)
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/v1/contacts/suppression", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerExactlyOneIdentifier(cmd, &email, &userID)
	return cmd
}

func (s *Service) newContactSuppressionRemoveCmd(key string) *cobra.Command {
	var email, userID string
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Un-suppress a contact by email or userId (DELETE /v1/contacts/suppression)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := singleIdentifierQuery(email, userID)
			resp, err := s.call(cmd.Context(), key, http.MethodDelete, "/v1/contacts/suppression", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerExactlyOneIdentifier(cmd, &email, &userID)
	return cmd
}

// registerExactlyOneIdentifier wires --email / --user-id and constrains them to
// exactly one (mutually exclusive + one required) — Loops' find / delete /
// suppression contract. A violation is a cobra usage error → exit 2.
func registerExactlyOneIdentifier(cmd *cobra.Command, email, userID *string) {
	cmd.Flags().StringVar(email, "email", "", "contact email address")
	cmd.Flags().StringVar(userID, "user-id", "", "external unique user id")
	cmd.MarkFlagsMutuallyExclusive("email", "user-id")
	cmd.MarkFlagsOneRequired("email", "user-id")
}

// singleIdentifierQuery returns the query for whichever identifier is set. The
// exactly-one constraint is already enforced by cobra, so exactly one is
// non-empty here.
func singleIdentifierQuery(email, userID string) url.Values {
	q := url.Values{}
	if email != "" {
		q.Set("email", email)
	} else {
		q.Set("userId", userID)
	}
	return q
}
