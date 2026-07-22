package notion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// newPageMoveCmd is `page move`: relocate each id to the resolved parent. Each
// id is typed (page vs database) and routed accordingly — a page via POST
// /v1/pages/{id}/move, a database via a parent update PATCH /v1/databases/{id}
// (databases have no /move endpoint). A data-source id is rejected (not
// independently movable). Fail-fast per id, reporting moved vs not.
func (s *Service) newPageMoveCmd(token string) *cobra.Command {
	var idsFlag, newParent string
	cmd := &cobra.Command{
		Use:   "move",
		Short: "Move pages or databases to a new parent",
		Args:  cobra.NoArgs,
		// POST /pages/{id}/move, PATCH /databases/{id}
		Annotations: map[string]string{"anycli.side_effect": "true"},
	}
	cmd.Flags().StringVar(&idsFlag, "page-or-database-ids", "", "JSON array of page or database ids to move (<=100)")
	cmd.Flags().StringVar(&newParent, "new-parent", "", "destination parent: JSON, URL, or id")
	_ = cmd.MarkFlagRequired("page-or-database-ids")
	_ = cmd.MarkFlagRequired("new-parent")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		raw, err := parseJSONFlag("page-or-database-ids", idsFlag)
		if err != nil {
			return err
		}
		var ids []string
		if err := json.Unmarshal(raw, &ids); err != nil {
			return &usageError{msg: "--page-or-database-ids must be a JSON array of id strings"}
		}
		if len(ids) == 0 {
			return &usageError{msg: "--page-or-database-ids is empty"}
		}
		if len(ids) > 100 {
			return &usageError{msg: "--page-or-database-ids accepts at most 100 ids (API limit)"}
		}
		parent, err := s.parentWire(cmd.Context(), token, newParent)
		if err != nil {
			return err
		}
		resolved := make([]string, len(ids))
		kinds := make([]string, len(ids))
		for i, id := range ids {
			rid, err := resolveID(id)
			if err != nil {
				return err
			}
			kind, err := s.probeIDType(cmd.Context(), token, rid)
			if errors.Is(err, errIndeterminateType) {
				return &usageError{msg: fmt.Sprintf(
					"move: could not resolve %s (check the id and that the integration has been granted access)", id)}
			}
			if err != nil {
				return err
			}
			// A data source isn't independently movable — it lives inside its
			// database container; move the database instead.
			if kind == "data_source" {
				return &usageError{msg: fmt.Sprintf("move accepts page or database ids; %s is a data source (move its parent database instead)", id)}
			}
			resolved[i] = rid
			kinds[i] = kind
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		var moved []string
		var bodies []json.RawMessage
		for i, rid := range resolved {
			var body []byte
			var err error
			switch kinds[i] {
			case "page":
				// Move a page: POST /v1/pages/{id}/move.
				body, err = s.call(cmd.Context(), token, http.MethodPost, "/pages/"+url.PathEscape(rid)+"/move", map[string]any{"parent": parent})
			case "database":
				// Move a database: databases have no /move endpoint — relocation
				// is a parent update via PATCH /v1/databases/{id} (the body's
				// `parent` field, verified against the update-a-database API).
				body, err = s.call(cmd.Context(), token, http.MethodPatch, "/databases/"+url.PathEscape(rid), map[string]any{"parent": parent})
			}
			if err != nil {
				fmt.Fprintf(s.stderr(), "moved %d/%d before failure (moved: [%s]); failed moving %s\n", len(moved), len(resolved), strings.Join(moved, ", "), ids[i])
				return err
			}
			moved = append(moved, rid)
			bodies = append(bodies, body)
		}
		if jsonMode {
			out, _ := json.Marshal(map[string]any{"moved": bodies})
			return s.emitJSON(out)
		}
		return s.emitLines(moved)
	}
	return cmd
}

// newPageDuplicateCmd is `page duplicate <page-id>`: POST /v1/pages with a
// top-level template. Without --new-parent it copies the source page's parent
// (the template endpoint does not inherit it). Async unless --allow-async polls.
func (s *Service) newPageDuplicateCmd(token string) *cobra.Command {
	var newParent, title string
	cmd := &cobra.Command{
		Use:   "duplicate <page-id>",
		Short: "Duplicate a page via a template",
		Args:  cobra.ExactArgs(1),
		// GET source + POST /pages
		Annotations: map[string]string{"anycli.side_effect": "true"},
	}
	cmd.Flags().StringVar(&newParent, "new-parent", "", "destination parent (default: source page's parent)")
	cmd.Flags().StringVar(&title, "title", "", "title for the duplicate (default: source title)")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		src, err := resolveID(args[0])
		if err != nil {
			return err
		}
		var parent json.RawMessage
		if strings.TrimSpace(newParent) != "" {
			if parent, err = s.parentWire(cmd.Context(), token, newParent); err != nil {
				return err
			}
		} else {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/pages/"+url.PathEscape(src), nil)
			if err != nil {
				return err
			}
			var pg struct {
				Parent json.RawMessage `json:"parent"`
			}
			if json.Unmarshal(body, &pg) != nil || len(pg.Parent) == 0 {
				return &apiError{msg: "page duplicate: cannot read the source page's parent"}
			}
			parent = pg.Parent
		}
		// POST /v1/pages `template` is a discriminated union: template_id is
		// honored only when type is "template_id" (the default "none" copies
		// nothing), so the type discriminator is required.
		payload := map[string]any{"parent": parent, "template": map[string]any{"type": "template_id", "template_id": src}}
		if strings.TrimSpace(title) != "" {
			// POST /v1/pages has no top-level `title`; a page title is a title
			// property under `properties` (the "title" key for a page parent).
			payload["properties"] = map[string]any{
				"title": map[string]any{
					"title": []any{
						map[string]any{"text": map[string]any{"content": title}},
					},
				},
			}
		}
		// A template-based create (POST /v1/pages with `template`) is always
		// synchronous and returns the new page directly. Notion rejects
		// `allow_async` unless a `markdown` body is present, so `page duplicate`
		// never forwards it — the global --allow-async is a no-op here.
		body, err := s.call(cmd.Context(), token, http.MethodPost, "/pages", payload)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		if jsonMode {
			return s.emitJSON(body)
		}
		ids := collectPageIDs(body)
		if len(ids) == 0 {
			return s.emitJSON(body)
		}
		return s.emitLines(ids[:1])
	}
	return cmd
}

// parentWire resolves --new-parent to the Notion parent wire object. Full JSON
// passes through; a URL is typed by slug; a bare uuid reuses the fetch probe
// (page → data_source). A database id or an all-fail probe is a usage error.
func (s *Service) parentWire(ctx context.Context, token, v string) (json.RawMessage, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, &usageError{msg: "--new-parent is empty"}
	}
	if strings.HasPrefix(v, "{") {
		return parseJSONFlag("new-parent", v)
	}
	if isURL(v) {
		id, err := resolveID(v)
		if err != nil {
			return nil, err
		}
		// A page URL types directly. Any other URL (a database/data-source view
		// URL) must be probed like a bare uuid — its extracted id is the
		// database container id, not a data_source id, so blindly emitting
		// data_source_id would carry the wrong id type.
		if detectURLType(v) == "page" {
			return parentIDWire("page_id", id), nil
		}
		return s.parentWireFromID(ctx, token, v, id)
	}
	id, err := resolveID(v)
	if err != nil {
		return nil, err
	}
	return s.parentWireFromID(ctx, token, v, id)
}

// parentWireFromID probes an id's type (reusing the fetch endpoint chain) and
// packs it into the matching parent wire. A data-source id types as
// data_source_id; a page id as page_id; a database container id is rejected
// (records live in its data sources — resolve via `fetch <db-id>`); an
// indeterminate id is a usage error.
func (s *Service) parentWireFromID(ctx context.Context, token, orig, id string) (json.RawMessage, error) {
	kind, err := s.probeIDType(ctx, token, id)
	if errors.Is(err, errIndeterminateType) {
		return nil, &usageError{msg: fmt.Sprintf(
			"cannot determine --new-parent type for %q; pass a page or data-source id/URL, or a full parent JSON object", orig)}
	}
	if err != nil {
		return nil, err
	}
	switch kind {
	case "page":
		return parentIDWire("page_id", id), nil
	case "data_source":
		return parentIDWire("data_source_id", id), nil
	default:
		return nil, &usageError{msg: fmt.Sprintf(
			"--new-parent %q is a database container, not a valid parent; to move/duplicate into a database pass a data-source id — run `fetch %s` and use one of its data_sources[] ids",
			orig, id)}
	}
}

// parentIDWire builds a {type, <field>:id} parent object.
func parentIDWire(field, id string) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"type": field, field: id})
	return b
}
