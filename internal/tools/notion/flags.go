package notion

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

// uuidRe matches a Notion id in dashed (8-4-4-4-12) or undashed 32-hex form.
var uuidRe = regexp.MustCompile(`[0-9a-fA-F]{8}-?[0-9a-fA-F]{4}-?[0-9a-fA-F]{4}-?[0-9a-fA-F]{4}-?[0-9a-fA-F]{12}`)

// resolveID accepts a bare id or a full Notion URL and returns the id. From a
// URL it extracts the trailing uuid; a bare uuid is normalized to dashed form;
// any other bare value passes through (Notion accepts both id forms). `self`
// is not special-cased here — that is local to user get.
func resolveID(idOrURL string) (string, error) {
	s := strings.TrimSpace(idOrURL)
	if s == "" {
		return "", &usageError{msg: "empty id"}
	}
	if isURL(s) {
		// The object id lives in the URL path tail; a database URL's view id
		// sits in the `v` query param and must not win, so scan the path only.
		scan := s
		if u, err := url.Parse(s); err == nil && u.Path != "" {
			scan = u.Path
		}
		matches := uuidRe.FindAllString(scan, -1)
		if len(matches) == 0 {
			return "", &usageError{msg: fmt.Sprintf("cannot extract a Notion id from URL %q", idOrURL)}
		}
		return normalizeID(matches[len(matches)-1]), nil
	}
	if uuidRe.FindString(s) == s {
		return normalizeID(s), nil
	}
	return s, nil
}

// normalizeID renders a 32-hex id in dashed 8-4-4-4-12 form; non-32-hex input
// is returned unchanged.
func normalizeID(raw string) string {
	h := strings.ReplaceAll(raw, "-", "")
	if len(h) != 32 {
		return raw
	}
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// isURL reports whether v looks like an http(s) URL.
func isURL(v string) bool {
	return strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://")
}

// hasNonASCII reports whether v contains any non-ASCII rune (an emoji signal).
func hasNonASCII(v string) bool {
	for _, r := range v {
		if r > 0x7F {
			return true
		}
	}
	return false
}

// readContent resolves single-segment markdown content from either an inline
// flag or --file. The two are mutually exclusive (fail-fast usage error); a
// file read error is also a usage error (bad path is a caller mistake).
func readContent(inline, file, inlineFlag string) (string, error) {
	if file != "" && inline != "" {
		return "", &usageError{msg: fmt.Sprintf("--file and --%s are mutually exclusive", inlineFlag)}
	}
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return "", &usageError{msg: fmt.Sprintf("read --file %s: %v", file, err)}
		}
		return string(b), nil
	}
	return inline, nil
}

// iconWire turns the --icon scalar sugar into the Notion wire object: an
// http(s) URL becomes an external file, anything else with a non-ASCII rune is
// treated as an emoji; a plain non-URL string is rejected.
func iconWire(v string) (json.RawMessage, error) {
	v = strings.TrimSpace(v)
	if isURL(v) {
		return externalWire(v), nil
	}
	if v != "" && hasNonASCII(v) {
		b, _ := json.Marshal(map[string]any{"type": "emoji", "emoji": v})
		return json.RawMessage(b), nil
	}
	return nil, &usageError{msg: fmt.Sprintf("--icon must be an emoji or an http(s) URL, got %q", v)}
}

// coverWire turns the --cover scalar sugar into the Notion wire object. Cover
// only accepts an http(s) URL.
func coverWire(v string) (json.RawMessage, error) {
	v = strings.TrimSpace(v)
	if isURL(v) {
		return externalWire(v), nil
	}
	return nil, &usageError{msg: fmt.Sprintf("--cover must be an http(s) URL, got %q", v)}
}

// externalWire builds the Notion external-file object for a URL.
func externalWire(link string) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"type": "external", "external": map[string]any{"url": link}})
	return json.RawMessage(b)
}

// parseJSONFlag validates a structured JSON flag value on parse. An empty value
// yields nil (flag unset); invalid JSON is a fail-fast usage error.
func parseJSONFlag(name, val string) (json.RawMessage, error) {
	if strings.TrimSpace(val) == "" {
		return nil, nil
	}
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(val), &raw); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("--%s is not valid JSON: %v", name, err)}
	}
	return raw, nil
}

// enumValidator returns a case-exact validator for a fixed value set. Distinct
// enum sets (fetch --type, search --type, view create --type) each build their
// own validator — they are intentionally not shared.
func enumValidator(flag string, allowed ...string) func(string) error {
	return func(v string) error {
		for _, a := range allowed {
			if v == a {
				return nil
			}
		}
		return &usageError{msg: fmt.Sprintf("--%s must be one of %s, got %q", flag, strings.Join(allowed, "|"), v)}
	}
}

// pageFlags holds the pagination flags shared by search / data-source query / comment
// list. They register locally on those commands only — never as global flags.
type pageFlags struct {
	all         bool
	pageSize    int
	startCursor string
}

// registerPaginationFlags attaches --all / --page-size / --start-cursor to cmd.
func registerPaginationFlags(cmd *cobra.Command) *pageFlags {
	pf := &pageFlags{}
	cmd.Flags().BoolVar(&pf.all, "all", false, "fetch every page by following next_cursor")
	cmd.Flags().IntVar(&pf.pageSize, "page-size", 0, "max items per page")
	cmd.Flags().StringVar(&pf.startCursor, "start-cursor", "", "resume from a prior response's next_cursor")
	return pf
}

// pageFetcher fetches one result page for a given start cursor.
type pageFetcher func(ctx context.Context, cursor string) ([]byte, error)

// paginate runs a list request. Without --all it returns the first page
// verbatim (has_more / next_cursor intact for manual continuation). With --all
// it follows next_cursor until has_more is false, accumulating results into a
// single list envelope.
func paginate(ctx context.Context, all bool, startCursor string, fetch pageFetcher) ([]byte, error) {
	if !all {
		return fetch(ctx, startCursor)
	}
	var acc []json.RawMessage
	cursor := startCursor
	for {
		body, err := fetch(ctx, cursor)
		if err != nil {
			return nil, err
		}
		var env struct {
			Results    []json.RawMessage `json:"results"`
			HasMore    bool              `json:"has_more"`
			NextCursor *string           `json:"next_cursor"`
		}
		if err := json.Unmarshal(body, &env); err != nil {
			return nil, &apiError{msg: fmt.Sprintf("notion: decode list page: %v", err), err: err}
		}
		acc = append(acc, env.Results...)
		if !env.HasMore || env.NextCursor == nil || *env.NextCursor == "" {
			break
		}
		cursor = *env.NextCursor
	}
	if acc == nil {
		acc = []json.RawMessage{}
	}
	return json.Marshal(map[string]any{"object": "list", "results": acc, "has_more": false, "next_cursor": nil})
}
