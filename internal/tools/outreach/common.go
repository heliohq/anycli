package outreach

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// resource pairs an Outreach path segment (plural, e.g. "prospects") with its
// JSON:API type name (singular, e.g. "prospect"). The path is used to build the
// URL; the type is used for sparse fieldsets and create/update bodies.
type resource struct {
	path string // plural, URL segment: "prospects"
	typ  string // singular JSON:API type: "prospect"
}

// --- Output flattening -----------------------------------------------------

// jsonAPIResource is one JSON:API resource object (data element).
type jsonAPIResource struct {
	ID            string                     `json:"id"`
	Type          string                     `json:"type"`
	Attributes    map[string]json.RawMessage `json:"attributes"`
	Relationships map[string]relationship    `json:"relationships"`
}

// relationship is a JSON:API relationship whose data may be a single linkage, an
// array of linkages, or null. RawMessage preserves that variability.
type relationship struct {
	Data json.RawMessage `json:"data"`
}

type linkage struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// flatten converts a JSON:API resource object to a provider-neutral map:
// {id, type, ...attributes, <rel>_id | <rel>_ids}. Relationship linkage ids are
// hoisted to top-level keys so agents never traverse the JSON:API envelope.
func flatten(raw json.RawMessage) (map[string]any, error) {
	var res jsonAPIResource
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	out := map[string]any{}
	if res.ID != "" {
		out["id"] = res.ID
	}
	if res.Type != "" {
		out["type"] = res.Type
	}
	for name, rawVal := range res.Attributes {
		var val any
		if err := json.Unmarshal(rawVal, &val); err != nil {
			return nil, fmt.Errorf("decode attribute %q: %w", name, err)
		}
		out[name] = val
	}
	for name, rel := range res.Relationships {
		hoistRelationship(out, name, rel.Data)
	}
	return out, nil
}

// hoistRelationship writes a relationship's linkage id(s) onto out under
// "<name>_id" (single) or "<name>_ids" (array). Null/absent data is skipped.
func hoistRelationship(out map[string]any, name string, data json.RawMessage) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return
	}
	if strings.HasPrefix(trimmed, "[") {
		var links []linkage
		if err := json.Unmarshal(data, &links); err != nil {
			return
		}
		ids := make([]string, 0, len(links))
		for _, l := range links {
			ids = append(ids, l.ID)
		}
		out[name+"_ids"] = ids
		return
	}
	var link linkage
	if err := json.Unmarshal(data, &link); err != nil || link.ID == "" {
		return
	}
	out[name+"_id"] = link.ID
}

// emitObject flattens a single-resource envelope ({"data": {...}}) and writes it
// as one JSON object.
func (s *Service) emitObject(body []byte) error {
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return &apiError{msg: fmt.Sprintf("outreach: decode response: %v", err), err: err}
	}
	obj, err := flatten(envelope.Data)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("outreach: decode resource: %v", err), err: err}
	}
	return s.emitValue(obj)
}

// listOutput is the flattened list envelope emitted by every list command.
type listOutput struct {
	Items      []map[string]any `json:"items"`
	NextCursor *string          `json:"next_cursor"`
	Count      *int             `json:"count,omitempty"`
}

// emitList flattens a collection envelope ({"data": [...], "links": {...},
// "meta": {...}}) to {items, next_cursor, count}. next_cursor is the page[after]
// token parsed from links.next (null when there is no next page); count is
// populated only when the response carries meta.count (i.e. --count was passed).
func (s *Service) emitList(body []byte) error {
	var envelope struct {
		Data  []json.RawMessage `json:"data"`
		Links struct {
			Next string `json:"next"`
		} `json:"links"`
		Meta struct {
			Count *int `json:"count"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return &apiError{msg: fmt.Sprintf("outreach: decode list: %v", err), err: err}
	}
	items := make([]map[string]any, 0, len(envelope.Data))
	for _, raw := range envelope.Data {
		obj, err := flatten(raw)
		if err != nil {
			return &apiError{msg: fmt.Sprintf("outreach: decode list item: %v", err), err: err}
		}
		items = append(items, obj)
	}
	out := listOutput{Items: items, NextCursor: nextCursor(envelope.Links.Next), Count: envelope.Meta.Count}
	return s.emitValue(out)
}

// nextCursor extracts the page[after] cursor token from a JSON:API links.next
// URL, returning nil when there is no next page or no cursor.
func nextCursor(next string) *string {
	if next == "" {
		return nil
	}
	u, err := url.Parse(next)
	if err != nil {
		return nil
	}
	cursor := u.Query().Get("page[after]")
	if cursor == "" {
		return nil
	}
	return &cursor
}

// emitValue marshals value and writes it to stdout with a trailing newline.
func (s *Service) emitValue(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("outreach: encode output: %v", err), err: err}
	}
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err = s.stdout().Write([]byte("\n"))
	return err
}

// --- List flags ------------------------------------------------------------

// listFlags holds the pagination / shaping flags shared by every list command.
type listFlags struct {
	limit  int
	cursor string
	fields string
	sort   string
	count  bool
}

// bindListFlags registers --limit/--cursor/--fields/--sort/--count on cmd. The
// values are read back with listFlagsFrom inside the command's RunE, keeping the
// registration decoupled from the closure.
func bindListFlags(cmd *cobra.Command) {
	cmd.Flags().Int("limit", 0, "page size (page[size]); 0 uses the Outreach default")
	cmd.Flags().String("cursor", "", "opaque page[after] cursor from a previous list's next_cursor")
	cmd.Flags().String("fields", "", "comma-separated sparse fieldset for the primary resource type")
	cmd.Flags().String("sort", "", "sort attribute; prefix with - for descending (e.g. -updatedAt)")
	cmd.Flags().Bool("count", false, "include the total match count in the output")
}

// listFlagsFrom reads the flags registered by bindListFlags back off cmd.
func listFlagsFrom(cmd *cobra.Command) *listFlags {
	lf := &listFlags{}
	lf.limit, _ = cmd.Flags().GetInt("limit")
	lf.cursor, _ = cmd.Flags().GetString("cursor")
	lf.fields, _ = cmd.Flags().GetString("fields")
	lf.sort, _ = cmd.Flags().GetString("sort")
	lf.count, _ = cmd.Flags().GetBool("count")
	return lf
}

// newGetCmd is the generic "get <id>" command shared by every resource.
func (s *Service) newGetCmd(token string, res resource) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get one " + res.typ + " by id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.runGet(cmd.Context(), token, res, args[0])
		},
	}
}

// newListCmd is the generic "list" command for resources with no resource-specific
// filter flags (reference/read-only resources). It exposes only the shared list
// flags.
func (s *Service) newListCmd(token string, res resource) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List " + res.path + " (one page)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			query := url.Values{}
			if err := listFlagsFrom(cmd).apply(query, res.typ); err != nil {
				return err
			}
			return s.runList(cmd.Context(), token, res, query)
		},
	}
	bindListFlags(cmd)
	return cmd
}

// setFilter sets filter[<key>]=value when value is non-empty.
func setFilter(q url.Values, key, value string) {
	if value != "" {
		q.Set("filter["+key+"]", value)
	}
}

// setRelFilter sets filter[<rel>][id]=value when value is non-empty (relationship
// filtering by id, the documented form since the May-2023 deprecation).
func setRelFilter(q url.Values, rel, value string) {
	if value != "" {
		q.Set("filter["+rel+"][id]", value)
	}
}

// setAttr writes a non-empty string attribute into attrs.
func setAttr(attrs map[string]any, key, value string) {
	if value != "" {
		attrs[key] = value
	}
}

// setRel writes a non-empty relationship id into rels.
func setRel(rels map[string]string, name, id string) {
	if id != "" {
		rels[name] = id
	}
}

// apply writes the list flags into a JSON:API query for the given resource type.
func (lf *listFlags) apply(q url.Values, typ string) error {
	if lf.limit < 0 {
		return &usageError{msg: "--limit must not be negative"}
	}
	if lf.limit > 0 {
		q.Set("page[size]", strconv.Itoa(lf.limit))
	}
	if lf.cursor != "" {
		q.Set("page[after]", lf.cursor)
	}
	if lf.fields != "" {
		q.Set("fields["+typ+"]", lf.fields)
	}
	if lf.sort != "" {
		q.Set("sort", lf.sort)
	}
	if lf.count {
		q.Set("count", "true")
	}
	return nil
}

// --- Request runners -------------------------------------------------------

// runList performs a list request and emits the flattened list envelope.
func (s *Service) runList(ctx context.Context, token string, res resource, q url.Values) error {
	body, err := s.call(ctx, token, http.MethodGet, "/"+res.path, q, nil)
	if err != nil {
		return err
	}
	return s.emitList(body)
}

// runGet performs a single-resource GET and emits the flattened object.
func (s *Service) runGet(ctx context.Context, token string, res resource, id string) error {
	if err := requireNumericID(res.typ+" id", id); err != nil {
		return err
	}
	body, err := s.call(ctx, token, http.MethodGet, "/"+res.path+"/"+url.PathEscape(id), nil, nil)
	if err != nil {
		return err
	}
	return s.emitObject(body)
}

// runCreate POSTs a JSON:API create body and emits the created resource.
func (s *Service) runCreate(ctx context.Context, token string, res resource, attrs map[string]any, rels map[string]string) error {
	payload := buildBody(res.typ, "", attrs, rels)
	body, err := s.call(ctx, token, http.MethodPost, "/"+res.path, nil, payload)
	if err != nil {
		return err
	}
	return s.emitObject(body)
}

// runUpdate PATCHes a JSON:API update body (which must carry type + id) and emits
// the updated resource.
func (s *Service) runUpdate(ctx context.Context, token string, res resource, id string, attrs map[string]any, rels map[string]string) error {
	if err := requireNumericID(res.typ+" id", id); err != nil {
		return err
	}
	payload := buildBody(res.typ, id, attrs, rels)
	body, err := s.call(ctx, token, http.MethodPatch, "/"+res.path+"/"+url.PathEscape(id), nil, payload)
	if err != nil {
		return err
	}
	return s.emitObject(body)
}

// runAction POSTs to /{path}/{id}/actions/{name} with optional actionParams and
// emits the returned resource. Outreach returns 200 with the resource body.
func (s *Service) runAction(ctx context.Context, token string, res resource, id, name string, actionParams url.Values) error {
	if err := requireNumericID(res.typ+" id", id); err != nil {
		return err
	}
	path := "/" + res.path + "/" + url.PathEscape(id) + "/actions/" + name
	body, err := s.call(ctx, token, http.MethodPost, path, actionParams, nil)
	if err != nil {
		return err
	}
	return s.emitObject(body)
}

// --- JSON:API body construction --------------------------------------------

// buildBody assembles a JSON:API request document. id is empty for create (the
// resource object must omit it) and set for update. rels maps a relationship
// name to a linkage id string (already validated as a relationship type via the
// caller's relationship map).
func buildBody(typ, id string, attrs map[string]any, rels map[string]string) map[string]any {
	data := map[string]any{"type": typ}
	if id != "" {
		data["id"] = id
	}
	if len(attrs) > 0 {
		data["attributes"] = attrs
	}
	if len(rels) > 0 {
		relationships := map[string]any{}
		for name, relID := range rels {
			relationships[name] = map[string]any{
				"data": map[string]any{"type": relationshipTypes[name], "id": relID},
			}
		}
		data["relationships"] = relationships
	}
	return map[string]any{"data": data}
}

// relationshipTypes maps a relationship name to the JSON:API type of its target.
// Most relationships share their attribute name with their type; the exception
// is owner, which is of type "user".
var relationshipTypes = map[string]string{
	"account":  "account",
	"prospect": "prospect",
	"sequence": "sequence",
	"mailbox":  "mailbox",
	"stage":    "stage",
	"owner":    "user",
}

// --- Attribute flags -------------------------------------------------------

// parseAttrs turns repeated --attr key=value flags into an attributes map. Each
// value is parsed as JSON when it is valid JSON (so numbers, booleans, arrays,
// and objects pass through with their real type); otherwise it is kept as a
// string. This lets an agent set any Outreach attribute without a dedicated flag.
func parseAttrs(pairs []string) (map[string]any, error) {
	attrs := map[string]any{}
	for _, pair := range pairs {
		key, raw, ok := strings.Cut(pair, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, &usageError{msg: fmt.Sprintf("invalid --attr %q: expected key=value", pair)}
		}
		var val any
		if err := json.Unmarshal([]byte(raw), &val); err != nil {
			val = raw
		}
		attrs[key] = val
	}
	return attrs, nil
}

// parseActionParams turns repeated key=value flags into a JSON:API
// actionParams[<key>] query. Values are kept as strings (action params are
// passed as query parameters, not JSON).
func parseActionParams(pairs []string) (url.Values, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	params := url.Values{}
	for _, pair := range pairs {
		key, val, ok := strings.Cut(pair, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, &usageError{msg: fmt.Sprintf("invalid --param %q: expected key=value", pair)}
		}
		params.Set("actionParams["+key+"]", val)
	}
	return params, nil
}

// --- Validation ------------------------------------------------------------

// requireNumericID validates that id is a non-empty positive integer, the shape
// of every Outreach resource id.
func requireNumericID(label, id string) error {
	if strings.TrimSpace(id) == "" {
		return &usageError{msg: label + " is required"}
	}
	if _, err := strconv.ParseInt(id, 10, 64); err != nil {
		return &usageError{msg: fmt.Sprintf("%s must be a numeric id, got %q", label, id)}
	}
	return nil
}
