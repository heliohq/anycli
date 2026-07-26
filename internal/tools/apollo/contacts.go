package apollo

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newContactsCmd builds the `contacts` group: persist a prospect into the
// team's Apollo DB (the prerequisite for sequencing), update stage, and list.
func (s *Service) newContactsCmd(token string) *cobra.Command {
	cmd := newGroupCmd("contacts", "Manage saved contacts")
	cmd.AddCommand(
		s.newContactsCreateCmd(token),
		s.newContactsBulkCreateCmd(token),
		s.newContactsUpdateCmd(token),
		s.newContactsSearchCmd(token),
		s.newContactsStagesCmd(token),
	)
	return cmd
}

// newContactsCreateCmd wraps POST /contacts.
func (s *Service) newContactsCreateCmd(token string) *cobra.Command {
	var body string
	var firstName, lastName, email, title, org, orgDomain string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a contact (POST /contacts)",
		Long: "Saving a contact is what makes a prospect sequenceable: `sequences add`\n" +
			"takes contact ids, not the person ids `people search` returns. This\n" +
			"command has no dedupe toggle (only `contacts bulk-create` carries\n" +
			"`--run-dedupe`), so check `contacts search` first when a duplicate record\n" +
			"would be a problem. Fields beyond name, email, title and company go\n" +
			"through `--body`.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, err := bodyFromFlag(body)
			if err != nil {
				return err
			}
			setStr(b, "first_name", firstName)
			setStr(b, "last_name", lastName)
			setStr(b, "email", email)
			setStr(b, "title", title)
			setStr(b, "organization_name", org)
			setStr(b, "domain", orgDomain)
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/contacts", nil, b)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&firstName, "first-name", "", "first name")
	cmd.Flags().StringVar(&lastName, "last-name", "", "last name")
	cmd.Flags().StringVar(&email, "email", "", "email address")
	cmd.Flags().StringVar(&title, "title", "", "job title")
	cmd.Flags().StringVar(&org, "org", "", "company name")
	cmd.Flags().StringVar(&orgDomain, "org-domain", "", "company domain")
	registerBodyFlag(cmd, &body)
	return cmd
}

// newContactsBulkCreateCmd wraps POST /contacts/bulk_create. Contacts are a raw
// JSON array via --contacts-json; --run-dedupe toggles Apollo's dedupe pass.
func (s *Service) newContactsBulkCreateCmd(token string) *cobra.Command {
	var body, contactsJSON string
	var runDedupe bool
	cmd := &cobra.Command{
		Use:   "bulk-create",
		Short: "Create up to 100 contacts in one call (POST /contacts/bulk_create)",
		Long: "--contacts-json takes a JSON ARRAY of up to 100 objects, each shaped like\n" +
			"the flags of `contacts create` (`first_name`, `last_name`, `email`,\n" +
			"`title`, `organization_name`, `domain`). --run-dedupe asks Apollo to\n" +
			"reconcile the batch against existing contacts instead of inserting\n" +
			"blindly, and it is only sent when the flag is explicitly set — omitting it\n" +
			"leaves Apollo's own default in force.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, err := bodyFromFlag(body)
			if err != nil {
				return err
			}
			if contactsJSON != "" {
				v, err := decodeJSONArray("contacts-json", contactsJSON)
				if err != nil {
					return err
				}
				b["contacts"] = v
			}
			if cmd.Flags().Changed("run-dedupe") {
				b["run_dedupe"] = runDedupe
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/contacts/bulk_create", nil, b)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&contactsJSON, "contacts-json", "", "JSON array of up to 100 contact objects")
	cmd.Flags().BoolVar(&runDedupe, "run-dedupe", false, "run Apollo dedupe against existing contacts")
	registerBodyFlag(cmd, &body)
	return cmd
}

// newContactsUpdateCmd wraps PATCH /contacts/{id}.
func (s *Service) newContactsUpdateCmd(token string) *cobra.Command {
	var body, title, email, stageID string
	cmd := &cobra.Command{
		Use:   "update <contact_id>",
		Short: "Update a contact (PATCH /contacts/{id})",
		Long: "Takes the contact id from `contacts create` or `contacts search` and\n" +
			"PATCHes only the fields given; omitted flags leave the stored value\n" +
			"untouched. --stage-id is an Apollo stage id, not a stage name — resolve it\n" +
			"with `contacts stages` first. Any other contact field goes through\n" +
			"`--body`.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := bodyFromFlag(body)
			if err != nil {
				return err
			}
			setStr(b, "title", title)
			setStr(b, "email", email)
			setStr(b, "contact_stage_id", stageID)
			resp, err := s.call(cmd.Context(), token, http.MethodPatch, "/contacts/"+url.PathEscape(args[0]), nil, b)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "job title")
	cmd.Flags().StringVar(&email, "email", "", "email address")
	cmd.Flags().StringVar(&stageID, "stage-id", "", "contact stage id (from `contacts stages`)")
	registerBodyFlag(cmd, &body)
	return cmd
}

// newContactsSearchCmd wraps POST /contacts/search.
func (s *Service) newContactsSearchCmd(token string) *cobra.Command {
	var body, q string
	var page, perPage int
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search existing contacts (POST /contacts/search)",
		Long: "Searches the contacts already saved in the team's Apollo workspace, not\n" +
			"Apollo's global prospect database — `people search` does that. --q is a\n" +
			"free-text keyword match across the record. Unlike `people search`, this\n" +
			"endpoint is reachable with the connected OAuth token.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, err := bodyFromFlag(body)
			if err != nil {
				return err
			}
			setStr(b, "q_keywords", q)
			applyPageBody(b, page, perPage)
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/contacts/search", nil, b)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&q, "q", "", "free-text keyword filter")
	registerPageFlags(cmd, &page, &perPage)
	registerBodyFlag(cmd, &body)
	return cmd
}

// newContactsStagesCmd wraps GET /contact_stages.
func (s *Service) newContactsStagesCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "stages",
		Short: "List contact stages (GET /contact_stages)",
		Long: "Returns the team's contact-stage ids alongside their names. Those ids are\n" +
			"what `contacts update --stage-id` expects; a stage name is not accepted\n" +
			"there, so resolve it here first. The stage set is configured per Apollo\n" +
			"team inside Apollo and cannot be created from this tool.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/contact_stages", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}
