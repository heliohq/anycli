package slides

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

func (s *Service) newTextInsertCmd(token string) *cobra.Command {
	var objectID, text string
	var at int
	var appendEnd bool
	cmd := &cobra.Command{
		Use:         "insert <presentation-id-or-url> --object <element-id> --text <string>",
		Short:       "Insert text into a shape/text box (default at the start; --at index or --append)",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if objectID == "" {
				return fmt.Errorf("slides: --object is required")
			}
			if text == "" {
				return fmt.Errorf("slides: --text is required")
			}
			pid := extractPresentationID(args[0])
			insertText := map[string]any{"objectId": objectID, "text": text}
			switch {
			case cmd.Flags().Changed("at"):
				insertText["insertionIndex"] = at
			case appendEnd:
				idx, err := s.appendIndex(cmd.Context(), token, pid, objectID)
				if err != nil {
					return err
				}
				insertText["insertionIndex"] = idx
			}
			requests := []any{map[string]any{"insertText": insertText}}
			respBody, err := s.batchUpdate(cmd.Context(), token, pid, requests)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(respBody)
			}
			fmt.Fprintf(s.stdout(), "inserted text into %s\n", objectID)
			return nil
		},
	}
	cmd.Flags().StringVar(&objectID, "object", "", "target text element object id (required)")
	cmd.Flags().StringVar(&text, "text", "", "text to insert (required)")
	cmd.Flags().IntVar(&at, "at", 0, "insertion index (UTF-16 code units)")
	cmd.Flags().BoolVar(&appendEnd, "append", false, "append at the end of the element's existing text")
	cmd.MarkFlagsMutuallyExclusive("at", "append")
	return cmd
}

// appendIndex fetches the deck, locates the element, and returns the index just
// before its trailing newline so inserted text lands at the end.
func (s *Service) appendIndex(ctx context.Context, token, pid, objectID string) (int, error) {
	body, err := s.call(ctx, token, http.MethodGet, "/presentations/"+pid, nil, nil)
	if err != nil {
		return 0, err
	}
	var p presentation
	if err := json.Unmarshal(body, &p); err != nil {
		return 0, fmt.Errorf("slides: decode presentation: %w", err)
	}
	txt, ok := p.findElementText(objectID)
	if !ok {
		return 0, fmt.Errorf("slides: element %q not found — run `pages get` to locate its object id", objectID)
	}
	n := utf16Len(txt)
	if n > 0 {
		return n - 1, nil
	}
	return 0, nil
}

func (s *Service) newTextReplaceCmd(token string) *cobra.Command {
	var find, replace string
	var matchCase bool
	var slides []string
	cmd := &cobra.Command{
		Use:         "replace <presentation-id-or-url> --find <string> --replace <string>",
		Short:       "Replace all occurrences of a string (the template-placeholder path: --find '{{name}}')",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if find == "" {
				return fmt.Errorf("slides: --find is required")
			}
			pid := extractPresentationID(args[0])
			replaceAll := map[string]any{
				"containsText": map[string]any{"text": find, "matchCase": matchCase},
				"replaceText":  replace,
			}
			if len(slides) > 0 {
				replaceAll["pageObjectIds"] = cleanIDs(slides)
			}
			requests := []any{map[string]any{"replaceAllText": replaceAll}}
			respBody, err := s.batchUpdate(cmd.Context(), token, pid, requests)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(respBody)
			}
			var resp batchUpdateResponse
			if err := json.Unmarshal(respBody, &resp); err != nil {
				return fmt.Errorf("slides: decode replace reply: %w", err)
			}
			changed := 0
			for _, r := range resp.Replies {
				if r.ReplaceAllText != nil {
					changed += r.ReplaceAllText.OccurrencesChanged
				}
			}
			fmt.Fprintf(s.stdout(), "replaced %d occurrence(s)\n", changed)
			return nil
		},
	}
	cmd.Flags().StringVar(&find, "find", "", "text to find (required)")
	cmd.Flags().StringVar(&replace, "replace", "", "replacement text")
	cmd.Flags().BoolVar(&matchCase, "match-case", false, "case-sensitive match")
	cmd.Flags().StringArrayVar(&slides, "slide", nil, "limit to these slide object ids (repeatable)")
	return cmd
}

func (s *Service) newTextDeleteCmd(token string) *cobra.Command {
	var objectID, textRange string
	cmd := &cobra.Command{
		Use:         "delete <presentation-id-or-url> --object <element-id>",
		Short:       "Delete text from an element (all text by default; --range A:B for a fixed span)",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if objectID == "" {
				return fmt.Errorf("slides: --object is required")
			}
			rng, err := parseTextRange(textRange)
			if err != nil {
				return err
			}
			pid := extractPresentationID(args[0])
			requests := []any{map[string]any{"deleteText": map[string]any{
				"objectId":  objectID,
				"textRange": rng,
			}}}
			respBody, err := s.batchUpdate(cmd.Context(), token, pid, requests)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(respBody)
			}
			fmt.Fprintf(s.stdout(), "deleted text from %s\n", objectID)
			return nil
		},
	}
	cmd.Flags().StringVar(&objectID, "object", "", "target text element object id (required)")
	cmd.Flags().StringVar(&textRange, "range", "", "index range in UTF-16 units: \"A:B\" (fixed), \"A:\" (from A), empty = all")
	return cmd
}

// parseTextRange maps the --range shorthand to a Slides Range object.
//   - ""    -> {type: ALL}
//   - "A:B" -> {type: FIXED_RANGE, startIndex: A, endIndex: B}
//   - "A:"  -> {type: FROM_START_INDEX, startIndex: A}
func parseTextRange(spec string) (map[string]any, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return map[string]any{"type": "ALL"}, nil
	}
	parts := strings.SplitN(spec, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("slides: --range must look like A:B or A:, got %q", spec)
	}
	start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || start < 0 {
		return nil, fmt.Errorf("slides: --range start must be a non-negative integer, got %q", parts[0])
	}
	endStr := strings.TrimSpace(parts[1])
	if endStr == "" {
		return map[string]any{"type": "FROM_START_INDEX", "startIndex": start}, nil
	}
	end, err := strconv.Atoi(endStr)
	if err != nil || end <= start {
		return nil, fmt.Errorf("slides: --range end must be an integer greater than start, got %q", parts[1])
	}
	return map[string]any{"type": "FIXED_RANGE", "startIndex": start, "endIndex": end}, nil
}
