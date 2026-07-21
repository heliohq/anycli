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
		Args:  cobra.NoArgs,
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
		Args:  cobra.ExactArgs(1),
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
		Args:  cobra.NoArgs,
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
		Args:  cobra.ExactArgs(1),
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
