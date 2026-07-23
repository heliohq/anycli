package salesloft

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newPersonCmd groups prospect (person) lookup and upkeep.
func (s *Service) newPersonCmd(token string) *cobra.Command {
	cmd := newGroupCmd("person", "Manage people (prospects)")
	cmd.AddCommand(
		s.newPersonListCmd(token),
		s.newPersonGetCmd(token),
		s.newPersonCreateCmd(token),
		s.newPersonUpdateCmd(token),
	)
	return cmd
}

func (s *Service) newPersonListCmd(token string) *cobra.Command {
	var lf listFlags
	var emails []string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List people (GET /v2/people); filter by --email or --updated-since",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q, err := lf.values()
			if err != nil {
				return err
			}
			for _, e := range emails {
				q.Add("email_addresses[]", e)
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/people", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerListFlags(cmd, &lf)
	cmd.Flags().StringArrayVar(&emails, "email", nil, "filter by email address (repeatable)")
	return cmd
}

func (s *Service) newPersonGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Fetch one person (GET /v2/people/{id})",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/people/"+id, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "person id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newPersonCreateCmd(token string) *cobra.Command {
	var email, firstName, lastName, title, body string
	var accountID, ownerID, personStageID int
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a person (POST /v2/people)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			named := map[string]any{}
			if email != "" {
				named["email_address"] = email
			}
			if firstName != "" {
				named["first_name"] = firstName
			}
			if lastName != "" {
				named["last_name"] = lastName
			}
			if title != "" {
				named["title"] = title
			}
			if cmd.Flags().Changed("account-id") {
				named["account_id"] = accountID
			}
			if cmd.Flags().Changed("owner-id") {
				named["owner_id"] = ownerID
			}
			if cmd.Flags().Changed("person-stage-id") {
				named["person_stage_id"] = personStageID
			}
			payload, err := mergeBody(named, body)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/people", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerPersonWriteFlags(cmd, &email, &firstName, &lastName, &title, &accountID, &ownerID, &personStageID, &body)
	return cmd
}

func (s *Service) newPersonUpdateCmd(token string) *cobra.Command {
	var id, email, firstName, lastName, title, body string
	var accountID, ownerID, personStageID int
	cmd := &cobra.Command{
		Use:         "update",
		Short:       "Update a person (PUT /v2/people/{id})",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			named := map[string]any{}
			if email != "" {
				named["email_address"] = email
			}
			if firstName != "" {
				named["first_name"] = firstName
			}
			if lastName != "" {
				named["last_name"] = lastName
			}
			if title != "" {
				named["title"] = title
			}
			if cmd.Flags().Changed("account-id") {
				named["account_id"] = accountID
			}
			if cmd.Flags().Changed("owner-id") {
				named["owner_id"] = ownerID
			}
			if cmd.Flags().Changed("person-stage-id") {
				named["person_stage_id"] = personStageID
			}
			payload, err := mergeBody(named, body)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPut, "/people/"+id, nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "person id")
	_ = cmd.MarkFlagRequired("id")
	registerPersonWriteFlags(cmd, &email, &firstName, &lastName, &title, &accountID, &ownerID, &personStageID, &body)
	return cmd
}

// registerPersonWriteFlags wires the shared person create/update body flags.
func registerPersonWriteFlags(cmd *cobra.Command, email, firstName, lastName, title *string, accountID, ownerID, personStageID *int, body *string) {
	cmd.Flags().StringVar(email, "email", "", "email address (unique lookup)")
	cmd.Flags().StringVar(firstName, "first-name", "", "first name")
	cmd.Flags().StringVar(lastName, "last-name", "", "last name")
	cmd.Flags().StringVar(title, "title", "", "job title")
	cmd.Flags().IntVar(accountID, "account-id", 0, "linked account id")
	cmd.Flags().IntVar(ownerID, "owner-id", 0, "owning user id")
	cmd.Flags().IntVar(personStageID, "person-stage-id", 0, "person stage id")
	cmd.Flags().StringVar(body, "body", "", "raw JSON body; keys override the named flags for full fidelity")
}
