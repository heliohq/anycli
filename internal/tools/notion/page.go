package notion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// pageUpdate is the fully-resolved input to a page update: content op (empty =
// properties-only), the content payload for that op, and the property changes.
// icon/cover are pre-wired in the RunE so a bad scalar fails before any PATCH.
type pageUpdate struct {
	id             string
	command        string // "" = properties-only mode
	restContent    string // new_str (replace) or content (insert), already read
	contentUpdates json.RawMessage
	position       string // insert only; "" defaults to end
	properties     json.RawMessage
	iconWire       json.RawMessage
	coverWire      json.RawMessage
	allowDeleting  bool
	allowAsync     bool
	jsonMode       bool
}

func (u pageUpdate) hasProps() bool {
	return u.properties != nil || u.iconWire != nil || u.coverWire != nil
}

// asyncEnvelope detects an async-task response: with allow_async, create /
// duplicate and large markdown writes return an `async_task` handle carrying
// the task id, a poll status_url, and the recommended poll_after_seconds.
type asyncEnvelope struct {
	Object           string `json:"object"`
	ID               string `json:"id"`
	Status           string `json:"status"`
	StatusURL        string `json:"status_url"`
	PollAfterSeconds int    `json:"poll_after_seconds"`
}

// resolveAsync handles a possibly-async response. An async_task handle with
// --allow-async is polled to completion (honoring poll_after_seconds); without
// it, the task id is surfaced on stderr (exit 0) so the caller can `task get`
// it. A non-async body passes through unchanged.
func (s *Service) resolveAsync(ctx context.Context, token string, body []byte, allowAsync bool) ([]byte, error) {
	var env asyncEnvelope
	if json.Unmarshal(body, &env) != nil || env.Object != "async_task" || env.ID == "" {
		return body, nil
	}
	if !allowAsync {
		fmt.Fprintf(s.stderr(), "note: operation is async; poll with: task get %s\n", env.ID)
		return body, nil
	}
	return s.pollTask(ctx, token, env.ID, env.PollAfterSeconds)
}

// emitLines writes one string per line to stdout.
func (s *Service) emitLines(lines []string) error {
	_, err := io.WriteString(s.stdout(), strings.Join(lines, "\n")+"\n")
	return err
}

// collectPageIDs pulls page ids out of a create / duplicate / poll response,
// accepting a {pages|results:[…]} envelope, a bare array, or a single object.
func collectPageIDs(body []byte) []string {
	var ids []string
	var env struct {
		Pages []struct {
			ID string `json:"id"`
		} `json:"pages"`
		Results []struct {
			ID string `json:"id"`
		} `json:"results"`
	}
	if json.Unmarshal(body, &env) == nil {
		for _, p := range env.Pages {
			if p.ID != "" {
				ids = append(ids, p.ID)
			}
		}
		for _, p := range env.Results {
			if p.ID != "" {
				ids = append(ids, p.ID)
			}
		}
	}
	if len(ids) == 0 {
		var arr []struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(body, &arr) == nil {
			for _, p := range arr {
				if p.ID != "" {
					ids = append(ids, p.ID)
				}
			}
		}
	}
	if len(ids) == 0 {
		var one struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(body, &one) == nil && one.ID != "" {
			ids = append(ids, one.ID)
		}
	}
	return ids
}

// newPageCreateCmd is `page create` (POST /v1/pages). The single input face is
// --pages (MCP verbatim, an object array); a bare markdown output lists created
// page ids, --json returns the full structured result.
func (s *Service) newPageCreateCmd(token string) *cobra.Command {
	var pagesFlag string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create one or more pages from a --pages JSON array",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().StringVar(&pagesFlag, "pages", "", "JSON array of page objects (parent/properties/content/icon/cover)")
	_ = cmd.MarkFlagRequired("pages")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		pages, err := parseJSONFlag("pages", pagesFlag)
		if err != nil {
			return err
		}
		var arr []json.RawMessage
		if err := json.Unmarshal(pages, &arr); err != nil {
			return &usageError{msg: "--pages must be a JSON array of page objects"}
		}
		if len(arr) == 0 {
			return &usageError{msg: "--pages must contain at least one page object"}
		}
		allowAsync, _ := cmd.Flags().GetBool("allow-async")
		jsonMode, _ := cmd.Flags().GetBool("json")
		// POST /v1/pages creates a single page: it requires a top-level `parent`
		// and does not accept a `pages` batch envelope. Fan out one request per
		// element (mirroring how the MCP create-pages tool fans out), spreading
		// each element's fields (parent/properties/content/icon/cover) at the
		// top level of its own request body and carrying --allow-async per call.
		var ids []string
		var bodies []json.RawMessage
		for i, el := range arr {
			var page map[string]any
			if err := json.Unmarshal(el, &page); err != nil {
				return &usageError{msg: fmt.Sprintf("--pages[%d] must be a JSON object", i)}
			}
			// MCP create-pages carries the page body in `content` as a markdown
			// string, but REST POST /v1/pages expects markdown text in the
			// `markdown` field — its `content`/`children` are block-object arrays.
			// Map a string `content` to `markdown` so MCP-verbatim input is not
			// rejected as malformed blocks (design 304 §page create). A non-string
			// `content` (an explicit block array) is left untouched.
			if c, ok := page["content"].(string); ok {
				page["markdown"] = c
				delete(page, "content")
			}
			// allow_async is only valid when a markdown body is present (Notion
			// rejects it otherwise), so only forward it for elements that carry
			// markdown content.
			if allowAsync {
				if _, hasMD := page["markdown"]; hasMD {
					page["allow_async"] = true
				}
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/pages", page)
			if err != nil {
				if len(ids) > 0 {
					fmt.Fprintf(s.stderr(), "created %d/%d page(s) before failure (created: [%s]); failed creating --pages[%d]\n",
						len(ids), len(arr), strings.Join(ids, ", "), i)
				}
				return err
			}
			if body, err = s.resolveAsync(cmd.Context(), token, body, allowAsync); err != nil {
				return err
			}
			bodies = append(bodies, body)
			ids = append(ids, collectPageIDs(body)...)
		}
		if jsonMode || len(ids) == 0 {
			out, _ := json.Marshal(map[string]any{"pages": bodies})
			return s.emitJSON(out)
		}
		return s.emitLines(ids)
	}
	return cmd
}

// newPageUpdateCmd is the canonical `page update <page-id>`: content op (via
// --command) and/or a properties-only change. The flag matrix is validated
// fail-fast (design 304 §④) before any request.
func (s *Service) newPageUpdateCmd(token string) *cobra.Command {
	var command, newStr, content, contentUpdatesFlag, position, at, propertiesFlag, icon, cover string
	cmd := &cobra.Command{
		Use:   "update <page-id>",
		Short: "Update a page's content and/or properties",
		Args:  cobra.ExactArgs(1),
	}
	f := cmd.Flags()
	f.StringVar(&command, "command", "", "content op: replace_content|update_content|insert_content (omit for properties-only)")
	f.StringVar(&newStr, "new-str", "", "replacement markdown (replace_content)")
	f.StringVar(&content, "content", "", "markdown to insert (insert_content)")
	f.StringVar(&contentUpdatesFlag, "content-updates", "", "JSON array of {old_str,new_str,replace_all_matches?} (update_content)")
	f.StringVar(&position, "position", "", "insert position: start|end (insert_content, default end)")
	f.StringVar(&at, "at", "", "alias for --position")
	f.StringVar(&propertiesFlag, "properties", "", "JSON page property values")
	f.StringVar(&icon, "icon", "", "emoji or http(s) URL")
	f.StringVar(&cover, "cover", "", "http(s) URL")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := validateUpdateFlags(cmd, command); err != nil {
			return err
		}
		id, err := resolveID(args[0])
		if err != nil {
			return err
		}
		contentUpdates, err := parseJSONFlag("content-updates", contentUpdatesFlag)
		if err != nil {
			return err
		}
		properties, err := parseJSONFlag("properties", propertiesFlag)
		if err != nil {
			return err
		}
		pos := position
		if cmd.Flags().Changed("at") {
			pos = at
		}
		file, _ := cmd.Flags().GetString("file")
		u, err := buildPageUpdate(id, command, newStr, content, contentUpdates, pos, properties, icon, cover, file)
		if err != nil {
			return err
		}
		s.applyUpdateFlags(cmd, &u)
		return s.executePageUpdate(cmd.Context(), token, u)
	}
	return cmd
}

// buildPageUpdate resolves content (inline or --file) and pre-wires icon/cover
// so scalar-sugar errors fail before any request.
func buildPageUpdate(id, command, newStr, content string, contentUpdates json.RawMessage, position string, properties json.RawMessage, icon, cover, file string) (pageUpdate, error) {
	u := pageUpdate{id: id, command: command, contentUpdates: contentUpdates, position: position, properties: properties}
	switch command {
	case "replace_content":
		c, err := readContent(newStr, file, "new-str")
		if err != nil {
			return u, err
		}
		u.restContent = c
	case "insert_content":
		if position != "" && position != "start" && position != "end" {
			return u, &usageError{msg: "--position must be start or end"}
		}
		c, err := readContent(content, file, "content")
		if err != nil {
			return u, err
		}
		u.restContent = c
	}
	if icon != "" {
		w, err := iconWire(icon)
		if err != nil {
			return u, err
		}
		u.iconWire = w
	}
	if cover != "" {
		w, err := coverWire(cover)
		if err != nil {
			return u, err
		}
		u.coverWire = w
	}
	return u, nil
}

// applyUpdateFlags copies the global write flags onto a pageUpdate.
func (s *Service) applyUpdateFlags(cmd *cobra.Command, u *pageUpdate) {
	u.allowAsync, _ = cmd.Flags().GetBool("allow-async")
	u.allowDeleting, _ = cmd.Flags().GetBool("allow-deleting-content")
	u.jsonMode, _ = cmd.Flags().GetBool("json")
}

// contentPayload builds the markdown-endpoint body. The CLI `command` maps to
// the REST field `type`, and the operation's params are nested under an object
// keyed by that type value (e.g. {"type":"replace_content","replace_content":
// {...}}) — the markdown endpoint (Notion-Version 2026-03-11) rejects a flat
// body. Only `allow_async` is top-level. --position is always sent (default
// end), never omitted.
func contentPayload(u pageUpdate) map[string]any {
	op := map[string]any{}
	switch u.command {
	case "replace_content":
		op["new_str"] = u.restContent
		if u.allowDeleting {
			op["allow_deleting_content"] = true
		}
	case "update_content":
		op["content_updates"] = u.contentUpdates
		if u.allowDeleting {
			op["allow_deleting_content"] = true
		}
	case "insert_content":
		op["content"] = u.restContent
		op["position"] = positionWire(u.position)
	}
	p := map[string]any{"type": u.command, u.command: op}
	if u.allowAsync {
		p["allow_async"] = true
	}
	return p
}

// positionWire wraps the position scalar into the wire object, defaulting to
// end. The field is always present so behavior does not depend on the endpoint
// default.
func positionWire(at string) json.RawMessage {
	t := at
	if t == "" {
		t = "end"
	}
	b, _ := json.Marshal(map[string]any{"type": t})
	return b
}

// executePageUpdate runs the content PATCH then the properties PATCH, fail-fast
// and in that order. On a properties failure after content already landed it
// writes the post-content markdown to stdout and returns a partial-success
// apiError (exit 1) — never a faked success, never a rollback.
func (s *Service) executePageUpdate(ctx context.Context, token string, u pageUpdate) error {
	var contentBody []byte
	if u.command != "" {
		body, err := s.writePageMarkdown(ctx, token, u.id, contentPayload(u))
		if err != nil {
			return err
		}
		if body, err = s.resolveAsync(ctx, token, body, u.allowAsync); err != nil {
			return err
		}
		contentBody = body
	}
	if u.hasProps() {
		propsBody, err := s.patchPageProps(ctx, token, u)
		if err != nil {
			if u.command != "" {
				_ = s.emitPageMarkdown(ctx, token, u.id, contentBody, true)
				return partialUpdateError(err)
			}
			return err
		}
		if u.jsonMode {
			return s.emitJSON(propsBody)
		}
		return s.emitPageMarkdown(ctx, token, u.id, contentBody, true)
	}
	if u.jsonMode {
		return s.emitJSON(contentBody)
	}
	return s.emitPageMarkdown(ctx, token, u.id, contentBody, true)
}

// patchPageProps applies --properties / --icon / --cover via PATCH /v1/pages/{id}.
func (s *Service) patchPageProps(ctx context.Context, token string, u pageUpdate) ([]byte, error) {
	body := map[string]any{}
	if u.properties != nil {
		body["properties"] = u.properties
	}
	if u.iconWire != nil {
		body["icon"] = u.iconWire
	}
	if u.coverWire != nil {
		body["cover"] = u.coverWire
	}
	return s.call(ctx, token, http.MethodPatch, "/pages/"+url.PathEscape(u.id), body)
}

// emitPageMarkdown writes the post-update markdown, preferring the PATCH
// response's markdown and falling back to a fresh GET when it lacks one (e.g. a
// properties-only update or an async result). When the mutation already
// succeeded (bestEffort), a failed confirmation read is non-fatal: the write
// stands, so the read failure is noted on stderr and nil is returned rather
// than turning a successful mutation into an exit-1 failure (design 304).
func (s *Service) emitPageMarkdown(ctx context.Context, token, id string, contentBody []byte, bestEffort bool) error {
	if len(contentBody) > 0 {
		var pm pageMarkdown
		if json.Unmarshal(contentBody, &pm) == nil && pm.Markdown != "" {
			return s.emitMarkdown(contentBody)
		}
	}
	body, err := s.readPageMarkdown(ctx, token, id)
	if err != nil {
		if bestEffort {
			fmt.Fprintf(s.stderr(),
				"note: the update succeeded but reading back the page markdown failed: %v; re-fetch to view the current content\n", err)
			return nil
		}
		return err
	}
	return s.emitMarkdown(body)
}

// partialUpdateError wraps a properties-stage failure, preserving the inner
// status and credential classification for errors.As.
func partialUpdateError(err error) error {
	status := 0
	var inner *apiError
	if errors.As(err, &inner) {
		status = inner.status
	}
	return &apiError{
		msg:    "page update: content was written but properties were not: " + err.Error(),
		status: status,
		err:    err,
	}
}

// validateUpdateFlags enforces the design-304 §④ --command × flags matrix for
// the canonical `page update` (aliases pin a command and cannot mis-combine).
func validateUpdateFlags(cmd *cobra.Command, command string) error {
	ch := func(n string) bool { return cmd.Flags().Changed(n) }
	newStr, content, updates := ch("new-str"), ch("content"), ch("content-updates")
	position := ch("position") || ch("at")
	file, del := ch("file"), ch("allow-deleting-content")
	props := ch("properties") || ch("icon") || ch("cover")

	forbid := func(set bool, name string) error {
		if set {
			return &usageError{msg: fmt.Sprintf("--%s is not allowed with --command %s", name, command)}
		}
		return nil
	}
	firstErr := func(errs ...error) error {
		for _, e := range errs {
			if e != nil {
				return e
			}
		}
		return nil
	}
	switch command {
	case "replace_content":
		if !newStr && !file {
			return &usageError{msg: "--command replace_content requires --new-str or --file"}
		}
		return firstErr(forbid(content, "content"), forbid(updates, "content-updates"), forbid(position, "position"))
	case "update_content":
		if !updates {
			return &usageError{msg: "--command update_content requires --content-updates"}
		}
		return firstErr(forbid(newStr, "new-str"), forbid(content, "content"), forbid(position, "position"), forbid(file, "file"))
	case "insert_content":
		if !content && !file {
			return &usageError{msg: "--command insert_content requires --content or --file"}
		}
		return firstErr(forbid(newStr, "new-str"), forbid(updates, "content-updates"), forbid(del, "allow-deleting-content"))
	case "":
		if !props {
			return &usageError{msg: "nothing to update: give --command with content, or --properties/--icon/--cover for a properties-only update"}
		}
		for _, c := range []struct {
			set  bool
			name string
		}{{newStr, "new-str"}, {content, "content"}, {updates, "content-updates"}, {position, "position"}, {file, "file"}, {del, "allow-deleting-content"}} {
			if c.set {
				return &usageError{msg: fmt.Sprintf("--%s is not allowed without --command (properties-only update)", c.name)}
			}
		}
		return nil
	default:
		return &usageError{msg: fmt.Sprintf("--command must be one of replace_content|update_content|insert_content, got %q", command)}
	}
}

// newPageReplaceCmd is the `page replace` alias (command=replace_content).
func (s *Service) newPageReplaceCmd(token string) *cobra.Command {
	var newStr string
	cmd := &cobra.Command{
		Use:   "replace <page-id>",
		Short: "Replace a page's entire content (alias for update --command replace_content)",
		Args:  cobra.ExactArgs(1),
	}
	cmd.Flags().StringVar(&newStr, "new-str", "", "replacement markdown (or --file)")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		id, err := resolveID(args[0])
		if err != nil {
			return err
		}
		if !cmd.Flags().Changed("new-str") && !cmd.Flags().Changed("file") {
			return &usageError{msg: "page replace requires --new-str or --file"}
		}
		file, _ := cmd.Flags().GetString("file")
		c, err := readContent(newStr, file, "new-str")
		if err != nil {
			return err
		}
		u := pageUpdate{id: id, command: "replace_content", restContent: c}
		s.applyUpdateFlags(cmd, &u)
		return s.executePageUpdate(cmd.Context(), token, u)
	}
	return cmd
}

// newPageEditCmd is the `page edit` alias: repeatable --old/--new zipped in
// order into content_updates (command=update_content). replace_all_matches is
// not exposed — use canonical --content-updates for that.
func (s *Service) newPageEditCmd(token string) *cobra.Command {
	var olds, news []string
	cmd := &cobra.Command{
		Use:   "edit <page-id>",
		Short: "Search-and-replace text in a page (alias for update --command update_content)",
		Args:  cobra.ExactArgs(1),
	}
	cmd.Flags().StringArrayVar(&olds, "old", nil, "text to find (repeatable, paired with --new)")
	cmd.Flags().StringArrayVar(&news, "new", nil, "replacement text (repeatable, paired with --old)")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		id, err := resolveID(args[0])
		if err != nil {
			return err
		}
		// update_content has no single-content input, so --file does not apply;
		// reject it rather than silently ignoring a supplied flag (§④ matrix).
		if cmd.Flags().Changed("file") {
			return &usageError{msg: "--file is not allowed with page edit (update_content takes --old/--new, not single-content input)"}
		}
		if len(olds) == 0 {
			return &usageError{msg: "page edit requires at least one --old/--new pair"}
		}
		if len(olds) != len(news) {
			return &usageError{msg: fmt.Sprintf("page edit: %d --old vs %d --new; counts must match", len(olds), len(news))}
		}
		updates := make([]map[string]string, len(olds))
		for i := range olds {
			updates[i] = map[string]string{"old_str": olds[i], "new_str": news[i]}
		}
		raw, _ := json.Marshal(updates)
		u := pageUpdate{id: id, command: "update_content", contentUpdates: raw}
		s.applyUpdateFlags(cmd, &u)
		return s.executePageUpdate(cmd.Context(), token, u)
	}
	return cmd
}

// newPageInsertCmd is the `page insert` alias (command=insert_content, --at).
func (s *Service) newPageInsertCmd(token string) *cobra.Command {
	var content, at string
	cmd := &cobra.Command{
		Use:   "insert <page-id>",
		Short: "Insert markdown at the start or end of a page (alias for update --command insert_content)",
		Args:  cobra.ExactArgs(1),
	}
	cmd.Flags().StringVar(&content, "content", "", "markdown to insert (or --file)")
	cmd.Flags().StringVar(&at, "at", "", "insert position: start|end (default end)")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		id, err := resolveID(args[0])
		if err != nil {
			return err
		}
		if !cmd.Flags().Changed("content") && !cmd.Flags().Changed("file") {
			return &usageError{msg: "page insert requires --content or --file"}
		}
		// insert_content does not accept allow_deleting_content (§④ matrix,
		// endpoint constraint); reject it rather than silently dropping it.
		if cmd.Flags().Changed("allow-deleting-content") {
			return &usageError{msg: "--allow-deleting-content is not allowed with page insert (insert_content does not accept it)"}
		}
		if at != "" && at != "start" && at != "end" {
			return &usageError{msg: "--at must be start or end"}
		}
		file, _ := cmd.Flags().GetString("file")
		c, err := readContent(content, file, "content")
		if err != nil {
			return err
		}
		u := pageUpdate{id: id, command: "insert_content", restContent: c, position: at}
		s.applyUpdateFlags(cmd, &u)
		return s.executePageUpdate(cmd.Context(), token, u)
	}
	return cmd
}

// newPageAppendCmd is the `page append` alias (insert_content, position=end).
func (s *Service) newPageAppendCmd(token string) *cobra.Command {
	var content string
	cmd := &cobra.Command{
		Use:   "append <page-id>",
		Short: "Append markdown to the end of a page (alias for update --command insert_content)",
		Args:  cobra.ExactArgs(1),
	}
	cmd.Flags().StringVar(&content, "content", "", "markdown to append (or --file)")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		id, err := resolveID(args[0])
		if err != nil {
			return err
		}
		if !cmd.Flags().Changed("content") && !cmd.Flags().Changed("file") {
			return &usageError{msg: "page append requires --content or --file"}
		}
		// insert_content does not accept allow_deleting_content (§④ matrix,
		// endpoint constraint); reject it rather than silently dropping it.
		if cmd.Flags().Changed("allow-deleting-content") {
			return &usageError{msg: "--allow-deleting-content is not allowed with page append (insert_content does not accept it)"}
		}
		file, _ := cmd.Flags().GetString("file")
		c, err := readContent(content, file, "content")
		if err != nil {
			return err
		}
		u := pageUpdate{id: id, command: "insert_content", restContent: c, position: "end"}
		s.applyUpdateFlags(cmd, &u)
		return s.executePageUpdate(cmd.Context(), token, u)
	}
	return cmd
}
