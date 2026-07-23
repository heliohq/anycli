package pipedrive

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// paginateStyle selects which pagination flags a list command exposes and how
// they map to query params.
type paginateStyle int

const (
	paginateNone   paginateStyle = iota // list returns everything (e.g. v1 users)
	paginateCursor                      // v2: --cursor + --limit → cursor, limit
	paginateOffset                      // v1: --start + --limit → start, limit
)

// filterFlag is a list-command query filter: a string flag whose value, when
// set, is copied verbatim into the request query under queryKey.
type filterFlag struct {
	flag     string
	queryKey string
	usage    string
}

// fieldKind is the JSON type a write flag serializes to in the request body.
type fieldKind int

const (
	fieldString fieldKind = iota
	fieldInt
	fieldFloat
	fieldBool
)

// fieldSpec is a create/update body field: a flag bound to an API key with a
// concrete JSON type. Only flags the caller explicitly set are included in the
// body, so partial updates never clobber unspecified fields.
type fieldSpec struct {
	flag   string
	apiKey string
	kind   fieldKind
	usage  string
}

// resource is one CRM entity's command family. It is pure data (path, verbs,
// filters, fields) driving the generic op builders below, so every entity gets
// an identical, convention-consistent surface with zero per-entity HTTP code.
type resource struct {
	c *caller

	word         string // command word, e.g. "deal"
	short        string // group help line
	path         string // API collection path, e.g. "/api/v2/deals"
	updateMethod string // http.MethodPatch (v2, leads) or http.MethodPut (notes)
	createVerb   string // create subcommand word; "" defaults to "create"
	paginate     paginateStyle

	filters []filterFlag // list-command query filters
	fields  []fieldSpec  // create/update body fields
}

// group assembles the resource's cobra group from the requested op builders.
func (r resource) group(ops ...*cobra.Command) *cobra.Command {
	g := newGroupCmd(r.word, r.short)
	g.AddCommand(ops...)
	return g
}

// listCmd builds "<word> list": query filters + pagination, GET the collection.
func (r resource) listCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List " + r.word + "s",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			for _, f := range r.filters {
				if cmd.Flags().Changed(f.flag) {
					v, _ := cmd.Flags().GetString(f.flag)
					q.Set(f.queryKey, v)
				}
			}
			r.addPagination(cmd, q)
			return r.c.run(cmd.Context(), http.MethodGet, r.path, q, nil)
		},
	}
	for _, f := range r.filters {
		cmd.Flags().String(f.flag, "", f.usage)
	}
	r.registerPagination(cmd)
	return cmd
}

// getCmd builds "<word> get <id>": GET a single record.
func (r resource) getCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "get <id>",
		Short:       "Get one " + r.word + " by id",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			return r.c.run(cmd.Context(), http.MethodGet, r.path+"/"+url.PathEscape(args[0]), nil, nil)
		},
	}
}

// createCmd builds "<word> create" (or a custom verb, e.g. note "add").
func (r resource) createCmd() *cobra.Command {
	verb := r.createVerb
	if verb == "" {
		verb = "create"
	}
	cmd := &cobra.Command{
		Use:         verb,
		Short:       verb + " a " + r.word,
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := buildBody(cmd, r.fields)
			if err != nil {
				return err
			}
			return r.c.run(cmd.Context(), http.MethodPost, r.path, nil, body)
		},
	}
	registerWriteFlags(cmd, r.fields)
	return cmd
}

// updateCmd builds "<word> update <id>": partial update via the entity's
// update method (PATCH for v2/leads, PUT for notes).
func (r resource) updateCmd() *cobra.Command {
	method := r.updateMethod
	if method == "" {
		method = http.MethodPatch
	}
	cmd := &cobra.Command{
		Use:         "update <id>",
		Short:       "Update a " + r.word,
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := buildBody(cmd, r.fields)
			if err != nil {
				return err
			}
			return r.c.run(cmd.Context(), method, r.path+"/"+url.PathEscape(args[0]), nil, body)
		},
	}
	registerWriteFlags(cmd, r.fields)
	return cmd
}

// deleteCmd builds "<word> delete <id>".
func (r resource) deleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "delete <id>",
		Short:       "Delete a " + r.word,
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			return r.c.run(cmd.Context(), http.MethodDelete, r.path+"/"+url.PathEscape(args[0]), nil, nil)
		},
	}
}

// searchCmd builds "<word> search --term": v2 entity search at /{path}/search.
func (r resource) searchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "search",
		Short:       "Search " + r.word + "s by term",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			term, _ := cmd.Flags().GetString("term")
			q.Set("term", term)
			for _, key := range []string{"fields", "cursor"} {
				if cmd.Flags().Changed(key) {
					v, _ := cmd.Flags().GetString(key)
					q.Set(key, v)
				}
			}
			if cmd.Flags().Changed("limit") {
				v, _ := cmd.Flags().GetString("limit")
				q.Set("limit", v)
			}
			return r.c.run(cmd.Context(), http.MethodGet, r.path+"/search", q, nil)
		},
	}
	cmd.Flags().String("term", "", "search term (required, min 2 chars)")
	cmd.Flags().String("fields", "", "comma-separated fields to search")
	cmd.Flags().String("limit", "", "max results")
	cmd.Flags().String("cursor", "", "pagination cursor")
	_ = cmd.MarkFlagRequired("term")
	return cmd
}

// registerPagination adds the pagination flags matching r.paginate.
func (r resource) registerPagination(cmd *cobra.Command) {
	switch r.paginate {
	case paginateCursor:
		cmd.Flags().String("cursor", "", "pagination cursor (from additional_data.next_cursor)")
		cmd.Flags().String("limit", "", "max entries to return")
	case paginateOffset:
		cmd.Flags().String("start", "", "pagination offset (0-based)")
		cmd.Flags().String("limit", "", "max entries to return")
	}
}

// addPagination copies the set pagination flags into the query.
func (r resource) addPagination(cmd *cobra.Command, q url.Values) {
	copyStringFlag(cmd, q, "limit", "limit")
	switch r.paginate {
	case paginateCursor:
		copyStringFlag(cmd, q, "cursor", "cursor")
	case paginateOffset:
		copyStringFlag(cmd, q, "start", "start")
	}
}

// copyStringFlag copies a set string flag into the query under queryKey.
func copyStringFlag(cmd *cobra.Command, q url.Values, flag, queryKey string) {
	if cmd.Flags().Changed(flag) {
		v, _ := cmd.Flags().GetString(flag)
		q.Set(queryKey, v)
	}
}

// registerWriteFlags declares the create/update body flags plus the --data
// raw-JSON escape hatch (arbitrary fields not covered by a typed flag).
func registerWriteFlags(cmd *cobra.Command, fields []fieldSpec) {
	cmd.Flags().String("data", "", "raw JSON body merged under typed flags (escape hatch for uncovered fields)")
	for _, f := range fields {
		switch f.kind {
		case fieldString:
			cmd.Flags().String(f.flag, "", f.usage)
		case fieldInt:
			cmd.Flags().Int64(f.flag, 0, f.usage)
		case fieldFloat:
			cmd.Flags().Float64(f.flag, 0, f.usage)
		case fieldBool:
			cmd.Flags().Bool(f.flag, false, f.usage)
		}
	}
}

// buildBody assembles the request body: start from --data JSON (if given), then
// overlay each typed flag the caller explicitly set (typed flags win). Only set
// flags are included, so updates stay partial.
func buildBody(cmd *cobra.Command, fields []fieldSpec) (map[string]any, error) {
	body := map[string]any{}
	if raw, _ := cmd.Flags().GetString("data"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &body); err != nil {
			return nil, &usageError{msg: "--data is not valid JSON: " + err.Error()}
		}
	}
	for _, f := range fields {
		if !cmd.Flags().Changed(f.flag) {
			continue
		}
		switch f.kind {
		case fieldString:
			v, _ := cmd.Flags().GetString(f.flag)
			body[f.apiKey] = v
		case fieldInt:
			v, _ := cmd.Flags().GetInt64(f.flag)
			body[f.apiKey] = v
		case fieldFloat:
			v, _ := cmd.Flags().GetFloat64(f.flag)
			body[f.apiKey] = v
		case fieldBool:
			v, _ := cmd.Flags().GetBool(f.flag)
			body[f.apiKey] = v
		}
	}
	return body, nil
}
