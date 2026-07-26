package copper

import (
	"net/http"
	"strconv"

	"github.com/spf13/cobra"
)

// crudResource describes one CRM record type that exposes the uniform Copper
// verb set. path is the API resource segment; name is the CLI command word.
type crudResource struct {
	name      string // CLI command word, e.g. "person"
	path      string // API segment, e.g. "people"
	short     string
	findEmail bool // people expose POST /people/fetch_by_email
	longs     crudLongs
}

// crudLongs carries one record type's per-verb Long text. The builders below
// fix each verb's mechanics identically for every record type, but the record
// types differ in meaning, so the prose is declared per resource here rather
// than generated from a template.
type crudLongs struct {
	list      string
	get       string
	create    string
	update    string
	del       string
	findEmail string
}

// crudResources is the closed set of record types with uniform CRUD. Copper
// models list/read as POST /{path}/search; create/update/delete are the REST
// verbs on /{path}[/{id}].
var crudResources = []crudResource{
	{
		name:      "person",
		path:      "people",
		short:     "Contacts (people)",
		findEmail: true,
		longs: crudLongs{
			list: "Searches the CONTACT records — Copper calls them `people`. `--name`,\n" +
				"`--email` and `--assignee-id` are the typed filters; tags, custom fields,\n" +
				"date ranges and sorting go in `--json-body`, which is merged field by field\n" +
				"over them. For one known address `person find-email` is the direct lookup\n" +
				"and skips paging entirely.",
			get: "`--id` is required and is Copper's NUMERIC person id, not an email address —\n" +
				"`person find-email` resolves one into the other. Returns the full contact:\n" +
				"its `emails[]` and `phone_numbers[]` arrays, the company it is linked to,\n" +
				"its assignee and its custom fields.",
			create: "`--json-body` is required and carries the whole record; there are no typed\n" +
				"field flags, because Copper's person schema is large and partly\n" +
				"account-specific. Emails are an array of objects\n" +
				"(`{\"emails\":[{\"email\":\"a@b.com\",\"category\":\"work\"}]}`), not a bare string.\n" +
				"Copper does not deduplicate on email, so run `person find-email` first\n" +
				"unless a duplicate contact is acceptable.",
			update: "`--id` and `--json-body` are both required, and only the fields present in\n" +
				"the body change. Array fields such as `emails` and `phone_numbers` are\n" +
				"REPLACED wholesale rather than appended to, so send the complete array\n" +
				"including the entries meant to survive.",
			del: "`--id` is required, and the removal is permanent — Copper exposes no archive\n" +
				"or soft-delete, and nothing here restores a deleted contact or the activity\n" +
				"history attached to it. When the record still has reference value, a status\n" +
				"or tag change through `person update` is the reversible alternative.",
			findEmail: "`--email` is required and matched EXACTLY, so a near miss returns nothing\n" +
				"rather than a suggestion. This is the only fetch-by-email endpoint in the\n" +
				"tool — company, lead and opportunity have none — and it is how an address\n" +
				"becomes the numeric id that `person get`, `person update` and an activity's\n" +
				"`parent` reference all need.",
		},
	},
	{
		name:  "company",
		path:  "companies",
		short: "Company records",
		longs: crudLongs{
			list: "Searches company records. `--name` is the filter that matters here;\n" +
				"`--assignee-id` applies too, and anything richer goes in `--json-body`,\n" +
				"merged over the typed flags. A contact's employer is a reference held on the\n" +
				"PERSON record, so listing a company's people means searching `person`, not\n" +
				"reading them from the company.",
			get: "`--id` is required and is Copper's numeric company id. Returns the address,\n" +
				"domain, assignee and custom fields. There is no fetch-by-domain or\n" +
				"fetch-by-name endpoint, so `company list --name` is the only way in from a\n" +
				"name.",
			create: "`--json-body` is required and carries the whole record; there are no typed\n" +
				"field flags. Copper does not deduplicate on name or domain, so search before\n" +
				"creating if a duplicate would be a problem. The id returned is what a\n" +
				"person's and an opportunity's `company_id` then reference.",
			update: "`--id` and `--json-body` are both required, and only the fields sent change.\n" +
				"Array fields are replaced rather than merged, so send the complete array.\n" +
				"Renaming a company re-links nothing: contacts and opportunities point at it\n" +
				"by id, so the rename simply propagates.",
			del: "`--id` is required and the deletion is permanent. Records that referenced\n" +
				"this company — people, opportunities — are NOT deleted with it; they lose\n" +
				"the link and are left orphaned rather than tidied, so clean them up first if\n" +
				"that matters.",
		},
	},
	{
		name:  "lead",
		path:  "leads",
		short: "Leads (top of funnel)",
		longs: crudLongs{
			list: "Searches leads, Copper's pre-qualification records. A lead is a SEPARATE\n" +
				"object from `person` and `company` and is not returned by their searches, so\n" +
				"an unqualified inbound contact will not be found by `person list`. `--name`,\n" +
				"`--email` and `--assignee-id` are typed; status, tags and date ranges go in\n" +
				"`--json-body`.",
			get: "`--id` is required. Returns the lead with its status, source and the contact\n" +
				"details captured before qualification. A converted lead keeps its record\n" +
				"here — the person, company and opportunity it produced are independent\n" +
				"records with their own ids.",
			create: "`--json-body` is required. A lead is the right home for an unqualified\n" +
				"inbound contact; creating a `person` instead drops an unvetted contact\n" +
				"straight into the CRM proper. `customer_source_id` must come from\n" +
				"`lookup customer-sources` — an invented id is rejected outright.",
			update: "`--id` and `--json-body` are both required and only the fields sent change.\n" +
				"This verb cannot CONVERT a lead: Copper's convert endpoint is not exposed\n" +
				"here, so turning a lead into a person, company and opportunity happens in\n" +
				"the Copper UI and this only edits the lead in place.",
			del: "`--id` is required and the deletion is permanent. Deleting a lead that has\n" +
				"already been converted does not touch the person, company or opportunity it\n" +
				"produced — those survive independently, which also means deleting the lead\n" +
				"loses only the original capture record.",
		},
	},
	{
		name:  "opportunity",
		path:  "opportunities",
		short: "Opportunities (deals)",
		longs: crudLongs{
			list: "Searches the deals. `--assignee-id` is the useful typed filter — note that\n" +
				"`--name` matches the DEAL name, not the customer's. Pipeline, stage, status\n" +
				"and close-date filters go through `--json-body`. Results carry\n" +
				"`pipeline_id` and `pipeline_stage_id` as bare numbers, which\n" +
				"`lookup pipelines` and `lookup pipeline-stages` turn into names.",
			get: "`--id` is required. Returns the deal's monetary value, close date and\n" +
				"status, with `pipeline_id`, `pipeline_stage_id` and `customer_source_id`\n" +
				"given as numeric ids and no names attached — the `lookup` commands are what\n" +
				"make them readable.",
			create: "`--json-body` is required and must name both a `pipeline_id` and a\n" +
				"`pipeline_stage_id` from `lookup pipelines` / `lookup pipeline-stages`; a\n" +
				"stage belonging to a different pipeline is rejected. Link the deal at\n" +
				"creation through `primary_contact_id` and `company_id`, since attaching them\n" +
				"afterwards costs another `opportunity update`.",
			update: "`--id` and `--json-body` are both required. Advancing a deal is a\n" +
				"`pipeline_stage_id` change and closing one means setting `status` to `Won`\n" +
				"or `Lost`; a lost deal also wants a `loss_reason_id` from\n" +
				"`lookup loss-reasons`. No stage history is kept by this verb — the previous\n" +
				"stage is simply overwritten.",
			del: "`--id` is required and it is permanent. Deleting a closed deal removes it\n" +
				"from the pipeline's history and therefore from any won/lost reporting built\n" +
				"on it, so setting `status` to `Lost` through `opportunity update` is almost\n" +
				"always the right move instead.",
		},
	},
	{
		name:  "task",
		path:  "tasks",
		short: "Follow-up tasks",
		longs: crudLongs{
			list: "Searches follow-up tasks. `--assignee-id` is the filter that answers \"what is\n" +
				"on someone's plate\"; due dates, completion state and the related record all\n" +
				"go through `--json-body`. A task hangs off a person, company, lead or\n" +
				"opportunity via its related-resource reference rather than standing alone.",
			get: "`--id` is required. Returns the due date, priority, completion status and the\n" +
				"related resource — a type plus an id — which is what connects the follow-up\n" +
				"back to the contact or deal it belongs to. That reference is an id only; the\n" +
				"record itself needs its own `get`.",
			create: "`--json-body` is required. Attach the task to a record with a related\n" +
				"resource of `{\"type\":\"opportunity\",\"id\":123}` (or `person`, `company`,\n" +
				"`lead`) — a task created without one is real but floats free and never\n" +
				"appears on any record's timeline. `assignee_id` comes from `user list`.",
			update: "`--id` and `--json-body` are both required. Completing a task is a field\n" +
				"change in the body, not a separate verb. Only the fields sent are touched,\n" +
				"so a partial payload will not silently clear a due date or reassign the\n" +
				"task.",
			del: "`--id` is required and the removal is permanent. For a task that was actually\n" +
				"done, marking it complete through `task update` keeps the record that the\n" +
				"follow-up happened; deleting erases the fact that it ever existed.",
		},
	},
}

// newResourceCmd builds the uniform CRUD command group for one record type.
func (s *Service) newResourceCmd(token string, r crudResource) *cobra.Command {
	group := newGroupCmd(r.name, r.short)
	group.AddCommand(
		s.newResourceListCmd(token, r),
		s.newResourceGetCmd(token, r),
		s.newResourceCreateCmd(token, r),
		s.newResourceUpdateCmd(token, r),
		s.newResourceDeleteCmd(token, r),
	)
	if r.findEmail {
		group.AddCommand(s.newResourceFindEmailCmd(token, r))
	}
	return group
}

// searchFlags holds the typed convenience filters assembled into the Copper
// POST /{path}/search body. --json-body overrides these field-by-field.
type searchFlags struct {
	name       string
	email      string
	assigneeID int
	page       int
	pageSize   int
	jsonBody   string
}

func registerSearchFlags(cmd *cobra.Command, f *searchFlags) {
	cmd.Flags().StringVar(&f.name, "name", "", "filter by name")
	cmd.Flags().StringVar(&f.email, "email", "", "filter by email address")
	cmd.Flags().IntVar(&f.assigneeID, "assignee-id", 0, "filter by assignee (Copper user id)")
	cmd.Flags().IntVar(&f.page, "page", 0, "page number (1-based)")
	cmd.Flags().IntVar(&f.pageSize, "page-size", 0, "results per page")
	cmd.Flags().StringVar(&f.jsonBody, "json-body", "", "raw JSON search body (merged over typed filters)")
}

// searchBody assembles the JSON search body from the typed filters, then merges
// the raw --json-body on top so an agent can express any Copper filter the
// typed flags don't cover (custom fields, date ranges, tags, …).
func (f searchFlags) searchBody() (map[string]any, error) {
	body := map[string]any{}
	if f.page > 0 {
		body["page_number"] = f.page
	}
	if f.pageSize > 0 {
		body["page_size"] = f.pageSize
	}
	if f.name != "" {
		body["name"] = f.name
	}
	if f.email != "" {
		body["emails"] = []string{f.email}
	}
	if f.assigneeID > 0 {
		body["assignee_ids"] = []int{f.assigneeID}
	}
	if f.jsonBody != "" {
		override, err := decodeJSONBody(f.jsonBody)
		if err != nil {
			return nil, err
		}
		for k, v := range override {
			body[k] = v
		}
	}
	return body, nil
}

func (s *Service) newResourceListCmd(token string, r crudResource) *cobra.Command {
	var f searchFlags
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "Search " + r.path + " (POST /" + r.path + "/search)",
		Long:        r.longs.list,
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := f.searchBody()
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/"+r.path+"/search", body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerSearchFlags(cmd, &f)
	return cmd
}

func (s *Service) newResourceGetCmd(token string, r crudResource) *cobra.Command {
	var id int
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get one " + r.name + " by id (GET /" + r.path + "/{id})",
		Long:        r.longs.get,
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if id <= 0 {
				return &usageError{msg: "--id is required"}
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/"+r.path+"/"+strconv.Itoa(id), nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&id, "id", 0, "Copper record id")
	return cmd
}

func (s *Service) newResourceCreateCmd(token string, r crudResource) *cobra.Command {
	var jsonBody string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a " + r.name + " (POST /" + r.path + ")",
		Long:        r.longs.create,
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if jsonBody == "" {
				return &usageError{msg: "--json-body is required (the record payload)"}
			}
			body, err := decodeJSONBody(jsonBody)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/"+r.path, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&jsonBody, "json-body", "", "raw JSON record payload")
	return cmd
}

func (s *Service) newResourceUpdateCmd(token string, r crudResource) *cobra.Command {
	var (
		id       int
		jsonBody string
	)
	cmd := &cobra.Command{
		Use:         "update",
		Short:       "Update a " + r.name + " (PUT /" + r.path + "/{id})",
		Long:        r.longs.update,
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if id <= 0 {
				return &usageError{msg: "--id is required"}
			}
			if jsonBody == "" {
				return &usageError{msg: "--json-body is required (the fields to update)"}
			}
			body, err := decodeJSONBody(jsonBody)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPut, "/"+r.path+"/"+strconv.Itoa(id), body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&id, "id", 0, "Copper record id")
	cmd.Flags().StringVar(&jsonBody, "json-body", "", "raw JSON of the fields to update")
	return cmd
}

func (s *Service) newResourceDeleteCmd(token string, r crudResource) *cobra.Command {
	var id int
	cmd := &cobra.Command{
		Use:         "delete",
		Short:       "Delete a " + r.name + " (DELETE /" + r.path + "/{id})",
		Long:        r.longs.del,
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if id <= 0 {
				return &usageError{msg: "--id is required"}
			}
			resp, err := s.call(cmd.Context(), token, http.MethodDelete, "/"+r.path+"/"+strconv.Itoa(id), nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&id, "id", 0, "Copper record id")
	return cmd
}

func (s *Service) newResourceFindEmailCmd(token string, r crudResource) *cobra.Command {
	var email string
	cmd := &cobra.Command{
		Use:         "find-email",
		Short:       "Find a " + r.name + " by email (POST /" + r.path + "/fetch_by_email)",
		Long:        r.longs.findEmail,
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if email == "" {
				return &usageError{msg: "--email is required"}
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/"+r.path+"/fetch_by_email", map[string]any{"email": email})
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "email address to look up")
	return cmd
}
