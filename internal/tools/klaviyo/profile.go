package klaviyo

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newProfileCmd builds the `profile` group: list/get/create/update plus the
// consent operations (subscribe/unsubscribe/suppress/unsuppress) wired in
// newProfileConsentCmds.
func (s *Service) newProfileCmd(token string) *cobra.Command {
	group := newGroupCmd("profile", "Manage customer profiles")
	group.AddCommand(
		s.newProfileListCmd(token),
		s.newProfileGetCmd(token),
		s.newProfileCreateCmd(token),
		s.newProfileUpdateCmd(token),
	)
	group.AddCommand(s.newProfileConsentCmds(token)...)
	return group
}

func (s *Service) newProfileListCmd(token string) *cobra.Command {
	f := &listFlags{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List profiles (GET /profiles), e.g. --filter 'equals(email,\"x@y.com\")'",
		Long: "The main lookup: `--filter 'equals(email,\"amy@example.com\")'` or\n" +
			"`equals(external_id,…)` is how an address becomes the profile id that\n" +
			"most other commands take. Consent and suppression state are NOT in the\n" +
			"default response — add `--param 'additional-fields[profile]=subscriptions'`\n" +
			"to see them. Cursor-paged with --page-size 1-100.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q, err := f.query("profile")
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/profiles", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	registerListFlags(cmd, f)
	return cmd
}

func (s *Service) newProfileGetCmd(token string) *cobra.Command {
	f := &listFlags{}
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get one profile (GET /profiles/{id})",
		Long: "Takes the profile id, not an email — resolve one with `profile list\n" +
			"--filter`. Subscription and suppression state are absent from the default\n" +
			"payload, so add `--param 'additional-fields[profile]=subscriptions'`\n" +
			"before concluding anything about whether this person can be mailed.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			q, err := f.query("profile")
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/profiles/"+url.PathEscape(args[0]), q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	registerListFlags(cmd, f)
	return cmd
}

func (s *Service) newProfileCreateCmd(token string) *cobra.Command {
	var email, phone, externalID, data string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a profile (POST /profiles) from --email/--phone/--external-id or --data",
		Long: "At least one of --email, --phone (E.164) or --external-id is required, or\n" +
			"a full --data body. Creating a profile subscribes it to NOTHING: a new\n" +
			"profile carries no marketing consent until `profile subscribe` runs.\n" +
			"Klaviyo rejects a create whose email already exists, so an existing person\n" +
			"is updated through `profile update` on their id instead.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := profileWriteBody("profile", "", email, phone, externalID, data)
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/profiles", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	registerProfileWriteFlags(cmd, &email, &phone, &externalID, &data)
	return cmd
}

func (s *Service) newProfileUpdateCmd(token string) *cobra.Command {
	var email, phone, externalID, data string
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a profile (PATCH /profiles/{id}) from --email/--phone/--external-id or --data",
		Long: "A patch keyed on the positional id: only the attributes passed are changed.\n" +
			"It cannot move consent — subscription and suppression state are changed\n" +
			"only by `profile subscribe`, `profile unsubscribe`, `profile suppress` and\n" +
			"`profile unsuppress`, never by writing an attribute here.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := profileWriteBody("profile", args[0], email, phone, externalID, data)
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodPatch, "/profiles/"+url.PathEscape(args[0]), nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	registerProfileWriteFlags(cmd, &email, &phone, &externalID, &data)
	return cmd
}

// registerProfileWriteFlags wires the shared create/update flags.
func registerProfileWriteFlags(cmd *cobra.Command, email, phone, externalID, data *string) {
	cmd.Flags().StringVar(email, "email", "", "profile email")
	cmd.Flags().StringVar(phone, "phone", "", "profile phone_number (E.164)")
	cmd.Flags().StringVar(externalID, "external-id", "", "profile external_id")
	cmd.Flags().StringVar(data, "data", "", "raw JSON:API request body (overrides the --email/--phone/--external-id shorthand)")
}

// profileWriteBody builds the create/update payload. --data wins verbatim when
// set; otherwise it constructs the single-resource envelope from the
// convenience flags, requiring at least one identifier.
func profileWriteBody(resourceType, id, email, phone, externalID, data string) (any, error) {
	if data != "" {
		return parseDataFlag(data)
	}
	attrs := compactAttrs(map[string]string{
		"email":        email,
		"phone_number": phone,
		"external_id":  externalID,
	})
	if len(attrs) == 0 {
		return nil, &usageError{msg: "provide at least one of --email/--phone/--external-id, or --data"}
	}
	return resourceBody(resourceType, id, attrs, nil), nil
}
