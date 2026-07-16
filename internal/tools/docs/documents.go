package docs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/spf13/cobra"
)

// suggestionsViewModes maps the --suggestions flag to the API enum.
var suggestionsViewModes = map[string]string{
	"inline":         "SUGGESTIONS_INLINE",
	"preview-accept": "PREVIEW_SUGGESTIONS_ACCEPTED",
	"preview-reject": "PREVIEW_WITHOUT_SUGGESTIONS",
}

func (s *Service) newDocumentsGetCmd(token string) *cobra.Command {
	var format, tab, suggestions string
	var allTabs bool
	cmd := &cobra.Command{
		Use:   "get <doc|url>",
		Short: "Fetch a document (default: rendered as markdown; --format text|json for alternatives)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if format != "md" && format != "text" && format != "json" {
				return fmt.Errorf("docs: --format must be md, text, or json, got %q", format)
			}
			viewMode := ""
			if suggestions != "" {
				vm, ok := suggestionsViewModes[suggestions]
				if !ok {
					return fmt.Errorf("docs: --suggestions must be inline, preview-accept, or preview-reject, got %q", suggestions)
				}
				viewMode = vm
			}
			id, err := extractDocumentID(args[0])
			if err != nil {
				return err
			}
			q := url.Values{}
			if viewMode != "" {
				q.Set("suggestionsViewMode", viewMode)
			}
			if allTabs || tab != "" {
				q.Set("includeTabsContent", "true")
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/documents/"+url.PathEscape(id), q, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) || format == "json" {
				return s.emit(body)
			}
			var doc apiDocument
			if err := json.Unmarshal(body, &doc); err != nil {
				return fmt.Errorf("docs: decode document: %w", err)
			}
			fmt.Fprint(s.stdout(), renderDocument(doc, format == "md", tab))
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "md", "output format: md, text, or json")
	cmd.Flags().BoolVar(&allTabs, "all-tabs", false, "include every tab's content (includeTabsContent)")
	cmd.Flags().StringVar(&tab, "tab", "", "render only the tab with this tab id")
	cmd.Flags().StringVar(&suggestions, "suggestions", "", "suggestions view: inline, preview-accept, or preview-reject")
	return cmd
}

func (s *Service) newDocumentsCreateCmd(token string) *cobra.Command {
	var title, bodyFile string
	cmd := &cobra.Command{
		Use:   "create --title T [--body-file <md>]",
		Short: "Create a document; --body-file writes markdown into the new document",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if title == "" {
				return fmt.Errorf("docs: --title is required")
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/documents", nil, map[string]any{"title": title})
			if err != nil {
				return err
			}
			var created apiDocument
			if err := json.Unmarshal(body, &created); err != nil {
				return fmt.Errorf("docs: decode created document: %w", err)
			}
			if bodyFile == "" {
				return s.reportCreated(cmd, body, created)
			}
			md, err := os.ReadFile(bodyFile)
			if err != nil {
				return fmt.Errorf("docs: read body file: %w", err)
			}
			reqs, warnings := markdownToRequests(string(md), 1, "", "")
			s.printWarnings(warnings)
			if _, err := s.batchUpdate(cmd.Context(), token, created.DocumentID, reqs); err != nil {
				// Non-atomic: the empty document already exists. Surface its
				// URL alongside the explicit failure so the caller can append
				// or tell the user — never swallow the half-built document.
				fmt.Fprintf(s.stdout(), "document created but body write failed: %s\n", docURL(created.DocumentID))
				return fmt.Errorf("docs: document %s created, but writing the body failed: %w", created.DocumentID, err)
			}
			return s.reportCreated(cmd, body, created)
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "document title (required)")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "markdown file to write into the new document body")
	return cmd
}

// reportCreated emits the create result: raw JSON with --json, else a summary
// with the document URL.
func (s *Service) reportCreated(cmd *cobra.Command, body []byte, created apiDocument) error {
	if jsonOut(cmd) {
		return s.emit(body)
	}
	fmt.Fprintf(s.stdout(), "created %q\nid:  %s\nurl: %s\n", created.Title, created.DocumentID, docURL(created.DocumentID))
	return nil
}

func (s *Service) newDocumentsAppendCmd(token string) *cobra.Command {
	var text, bodyFile, tab string
	cmd := &cobra.Command{
		Use:   "append <doc|url> (--text S | --body-file <md>)",
		Short: "Append text or markdown to the end of a document (no index required)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if (text == "") == (bodyFile == "") {
				return fmt.Errorf("docs: exactly one of --text or --body-file is required")
			}
			id, err := extractDocumentID(args[0])
			if err != nil {
				return err
			}
			if text != "" {
				return s.appendText(cmd, token, id, text, tab)
			}
			return s.appendMarkdown(cmd, token, id, bodyFile, tab)
		},
	}
	cmd.Flags().StringVar(&text, "text", "", "plain text to append (inserted at the end of the segment)")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "markdown file to append (translated to batchUpdate requests)")
	cmd.Flags().StringVar(&tab, "tab", "", "target tab id (defaults to the first tab)")
	cmd.MarkFlagsMutuallyExclusive("text", "body-file")
	return cmd
}

// appendText inserts plain text at the end of a segment — index-free, no read
// required, concurrency safe.
func (s *Service) appendText(cmd *cobra.Command, token, id, text, tab string) error {
	loc := map[string]any{}
	if tab != "" {
		loc["tabId"] = tab
	}
	reqs := []map[string]any{
		{"insertText": map[string]any{"text": text, "endOfSegmentLocation": loc}},
	}
	body, err := s.batchUpdate(cmd.Context(), token, id, reqs)
	if err != nil {
		return err
	}
	return s.reportBatch(cmd, body, "appended text")
}

// appendMarkdown appends a markdown file. It reads the document once to find
// the body's end index (the tool computes the index; the caller never does),
// then inserts a fresh line plus the rendered content.
func (s *Service) appendMarkdown(cmd *cobra.Command, token, id, bodyFile, tab string) error {
	md, err := os.ReadFile(bodyFile)
	if err != nil {
		return fmt.Errorf("docs: read body file: %w", err)
	}
	insertIndex, err := s.segmentEndIndex(cmd.Context(), token, id, tab)
	if err != nil {
		return err
	}
	reqs, warnings := markdownToRequests(string(md), insertIndex, "\n", tab)
	s.printWarnings(warnings)
	if len(reqs) == 0 {
		return fmt.Errorf("docs: body file is empty")
	}
	body, err := s.batchUpdate(cmd.Context(), token, id, reqs)
	if err != nil {
		return err
	}
	return s.reportBatch(cmd, body, "appended markdown")
}

// segmentEndIndex reads the document and returns the last valid insertion index
// of the target body (the final newline position). A fresh/empty body yields 1.
func (s *Service) segmentEndIndex(ctx context.Context, token, id, tab string) (int, error) {
	q := url.Values{}
	if tab != "" {
		q.Set("includeTabsContent", "true")
	}
	body, err := s.call(ctx, token, http.MethodGet, "/documents/"+url.PathEscape(id), q, nil)
	if err != nil {
		return 0, err
	}
	var doc apiDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return 0, fmt.Errorf("docs: decode document: %w", err)
	}
	content := doc.Body.Content
	if tab != "" {
		if b, ok := findTabBody(doc.Tabs, tab); ok {
			content = b.Content
		} else {
			return 0, fmt.Errorf("docs: tab %q not found in document", tab)
		}
	}
	end := 1
	for _, el := range content {
		if el.EndIndex > end {
			end = el.EndIndex
		}
	}
	if end <= 1 {
		return 1, nil
	}
	return end - 1, nil
}

// findTabBody locates a tab's body by id, recursing into child tabs.
func findTabBody(tabs []apiTab, tabID string) (apiBody, bool) {
	for _, tab := range tabs {
		if tab.TabProperties.TabID == tabID && tab.DocumentTab != nil {
			return tab.DocumentTab.Body, true
		}
		if b, ok := findTabBody(tab.ChildTabs, tabID); ok {
			return b, true
		}
	}
	return apiBody{}, false
}

func (s *Service) newDocumentsReplaceAllCmd(token string) *cobra.Command {
	var find, replace string
	var matchCase bool
	cmd := &cobra.Command{
		Use:   "replace-all <doc|url> --find X --replace Y",
		Short: "Replace every occurrence of a string (reports occurrencesChanged)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if find == "" {
				return fmt.Errorf("docs: --find is required")
			}
			id, err := extractDocumentID(args[0])
			if err != nil {
				return err
			}
			reqs := []map[string]any{
				{"replaceAllText": map[string]any{
					"replaceText":  replace,
					"containsText": map[string]any{"text": find, "matchCase": matchCase},
				}},
			}
			body, err := s.batchUpdate(cmd.Context(), token, id, reqs)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			changed := occurrencesChanged(body)
			fmt.Fprintf(s.stdout(), "replaced %d occurrence(s) of %q\n", changed, find)
			return nil
		},
	}
	cmd.Flags().StringVar(&find, "find", "", "text to find (required)")
	cmd.Flags().StringVar(&replace, "replace", "", "replacement text (empty deletes the matches)")
	cmd.Flags().BoolVar(&matchCase, "match-case", false, "case-sensitive match")
	return cmd
}

func (s *Service) newDocumentsBatchUpdateCmd(token string) *cobra.Command {
	var requestsFile string
	cmd := &cobra.Command{
		Use:   "batch-update <doc|url> --requests-file <req.json>",
		Short: "Escape hatch: apply raw Docs API batchUpdate requests verbatim",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if requestsFile == "" {
				return fmt.Errorf("docs: --requests-file is required")
			}
			id, err := extractDocumentID(args[0])
			if err != nil {
				return err
			}
			raw, err := os.ReadFile(requestsFile)
			if err != nil {
				return fmt.Errorf("docs: read requests file: %w", err)
			}
			requests, err := parseRequests(raw)
			if err != nil {
				return err
			}
			body, err := s.batchUpdate(cmd.Context(), token, id, requests)
			if err != nil {
				return err
			}
			return s.reportBatch(cmd, body, "applied batch update")
		},
	}
	cmd.Flags().StringVar(&requestsFile, "requests-file", "", "JSON file: a requests array or a {\"requests\":[...]} object")
	return cmd
}

// parseRequests accepts either a bare JSON array of Request objects or a
// {"requests":[...]} envelope, returning the request list.
func parseRequests(raw []byte) ([]map[string]any, error) {
	var envelope struct {
		Requests []map[string]any `json:"requests"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && envelope.Requests != nil {
		return envelope.Requests, nil
	}
	var list []map[string]any
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("docs: requests file must be a JSON array or a {\"requests\":[...]} object: %w", err)
	}
	return list, nil
}

// batchUpdate posts a request list to documents.batchUpdate.
func (s *Service) batchUpdate(ctx context.Context, token, id string, requests []map[string]any) ([]byte, error) {
	payload := map[string]any{"requests": requests}
	return s.call(ctx, token, http.MethodPost, "/documents/"+url.PathEscape(id)+":batchUpdate", nil, payload)
}

// reportBatch emits the batchUpdate result: raw JSON with --json, else a short
// summary.
func (s *Service) reportBatch(cmd *cobra.Command, body []byte, summary string) error {
	if jsonOut(cmd) {
		return s.emit(body)
	}
	fmt.Fprintf(s.stdout(), "%s\n", summary)
	return nil
}

// occurrencesChanged pulls the replaceAllText occurrence count out of a
// batchUpdate response.
func occurrencesChanged(body []byte) int {
	var resp struct {
		Replies []struct {
			ReplaceAllText struct {
				OccurrencesChanged int `json:"occurrencesChanged"`
			} `json:"replaceAllText"`
		} `json:"replies"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0
	}
	total := 0
	for _, r := range resp.Replies {
		total += r.ReplaceAllText.OccurrencesChanged
	}
	return total
}

// printWarnings writes markdown-degradation warnings to stderr.
func (s *Service) printWarnings(warnings []string) {
	for _, w := range warnings {
		fmt.Fprintln(s.stderr(), "warning: "+w)
	}
}
