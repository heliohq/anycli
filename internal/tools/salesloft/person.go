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
		Use:   "list",
		Short: "List people (GET /v2/people); filter by --email or --updated-since",
		Long: "The lookup that belongs before any prospect write. `--email` is\n" +
			"repeatable, maps to Salesloft's `email_addresses[]` filter and matches\n" +
			"the whole address exactly — there is no partial or domain match — which\n" +
			"makes it the way to tell whether a prospect already exists. For an\n" +
			"incremental read use `--updated-since` with\n" +
			"`--sort-by updated_at --sort-direction ASC`; re-walking every page\n" +
			"spends the team's shared rate budget instead. Anything else goes through\n" +
			"`--filter`, e.g. `--filter \"account_id[]=77\"`.",
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
		Use:   "get",
		Short: "Fetch one person (GET /v2/people/{id})",
		Long: "`--id` is required and is Salesloft's integer person id from\n" +
			"`person list`; an email address is not accepted here, so start from\n" +
			"`person list --email` when that is all you have. Returns the whole\n" +
			"prospect record — contact details, `account_id`, `owner_id`,\n" +
			"`person_stage`, custom fields and the engagement counters.",
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
		Use:   "create",
		Short: "Create a person (POST /v2/people)",
		Long: "The CLI requires no field, but a person without `--email` cannot be\n" +
			"enrolled in an email cadence, so treat it as mandatory. Salesloft does\n" +
			"not deduplicate: run `person list --email` first, because a second\n" +
			"record on an address that already exists splits that prospect's history\n" +
			"and nothing here merges them. `--account-id` links the person to a\n" +
			"company, `--owner-id` assigns a rep from `user list`, and `--body`\n" +
			"carries anything with no flag, its keys overriding the named ones.",
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
		Use:   "update",
		Short: "Update a person (PUT /v2/people/{id})",
		Long: "`--id` is required. The request is partial despite being a PUT: only the\n" +
			"flags actually passed are sent, unmentioned fields keep their values,\n" +
			"and no named flag can blank one out. `--body` is the route to custom\n" +
			"fields and anything else without a flag —\n" +
			"`--body '{\"custom_fields\":{\"Region\":\"EU\"}}'` — and its keys win over\n" +
			"overlapping named flags. Changing `--email` re-points the record; it\n" +
			"does not create a second one.",
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
