package notion

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// newPageMoveCmd is `page move`: loop each page id through POST
// /v1/pages/{page_id}/move with the resolved parent. All ids are prechecked to
// be pages (database/data-source ids have no move endpoint → usage error).
func (s *Service) newPageMoveCmd(token string) *cobra.Command {
	var idsFlag, newParent string
	cmd := &cobra.Command{
		Use:   "move",
		Short: "Move pages to a new parent",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().StringVar(&idsFlag, "page-or-database-ids", "", "JSON array of page ids to move (<=100)")
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
		for i, id := range ids {
			rid, err := resolveID(id)
			if err != nil {
				return err
			}
			kind, err := s.probeIDType(cmd.Context(), token, rid)
			if err != nil {
				return err
			}
			if kind != "page" {
				return &usageError{msg: fmt.Sprintf("move only supports page ids; %s is a %s (database/data-source move has no standard REST endpoint)", id, kind)}
			}
			resolved[i] = rid
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		var moved []string
		var bodies []json.RawMessage
		for i, rid := range resolved {
			body, err := s.callWithVersion(cmd.Context(), token, http.MethodPost, "/pages/"+url.PathEscape(rid)+"/move", map[string]any{"parent": parent}, markdownVersion)
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
			body, err := s.callWithVersion(cmd.Context(), token, http.MethodGet, "/pages/"+url.PathEscape(src), nil, markdownVersion)
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
		payload := map[string]any{"parent": parent, "template": map[string]any{"template_id": src}}
		if strings.TrimSpace(title) != "" {
			payload["title"] = title
		}
		allowAsync, _ := cmd.Flags().GetBool("allow-async")
		if allowAsync {
			payload["allow_async"] = true
		}
		body, err := s.callWithVersion(cmd.Context(), token, http.MethodPost, "/pages", payload, markdownVersion)
		if err != nil {
			return err
		}
		if body, err = s.resolveAsync(cmd.Context(), token, body, allowAsync); err != nil {
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
		if detectURLType(v) == "page" {
			return parentIDWire("page_id", id), nil
		}
		return parentIDWire("data_source_id", id), nil
	}
	id, err := resolveID(v)
	if err != nil {
		return nil, err
	}
	kind, err := s.probeIDType(ctx, token, id)
	if err != nil {
		return nil, err
	}
	switch kind {
	case "page":
		return parentIDWire("page_id", id), nil
	case "data_source":
		return parentIDWire("data_source_id", id), nil
	default:
		return nil, &usageError{msg: fmt.Sprintf("cannot determine --new-parent type for %q", v)}
	}
}

// parentIDWire builds a {type, <field>:id} parent object.
func parentIDWire(field, id string) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"type": field, field: id})
	return b
}
