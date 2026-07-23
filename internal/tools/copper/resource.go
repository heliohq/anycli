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
}

// crudResources is the closed set of record types with uniform CRUD. Copper
// models list/read as POST /{path}/search; create/update/delete are the REST
// verbs on /{path}[/{id}].
var crudResources = []crudResource{
	{name: "person", path: "people", short: "Contacts (people)", findEmail: true},
	{name: "company", path: "companies", short: "Company records"},
	{name: "lead", path: "leads", short: "Leads (top of funnel)"},
	{name: "opportunity", path: "opportunities", short: "Opportunities (deals)"},
	{name: "task", path: "tasks", short: "Follow-up tasks"},
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
		Use:   "list",
		Short: "Search " + r.path + " (POST /" + r.path + "/search)",
		Args:  cobra.NoArgs,
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
		Use:   "get",
		Short: "Get one " + r.name + " by id (GET /" + r.path + "/{id})",
		Args:  cobra.NoArgs,
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
		Use:   "create",
		Short: "Create a " + r.name + " (POST /" + r.path + ")",
		Args:  cobra.NoArgs,
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
		Use:   "update",
		Short: "Update a " + r.name + " (PUT /" + r.path + "/{id})",
		Args:  cobra.NoArgs,
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
		Use:   "delete",
		Short: "Delete a " + r.name + " (DELETE /" + r.path + "/{id})",
		Args:  cobra.NoArgs,
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
		Use:   "find-email",
		Short: "Find a " + r.name + " by email (POST /" + r.path + "/fetch_by_email)",
		Args:  cobra.NoArgs,
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
