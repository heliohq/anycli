package slides

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// newSlidesAddCmd creates a slide and, in the same atomic batchUpdate, fills
// its TITLE / BODY placeholders. createSlide alone only yields empty
// placeholders; assigning their ids via placeholderIdMappings up front lets a
// single call also insertText into them (design 303 — the objectId bookkeeping
// stays in the tool, the AI only supplies content).
func (s *Service) newSlidesAddCmd(token string) *cobra.Command {
	var layout, title, body string
	var at int
	cmd := &cobra.Command{
		Use:   "add <presentation-id-or-url>",
		Short: "Add a slide (optionally filling its title/body); --layout takes a PredefinedLayout",
		Long: "--title / --body fill the layout's TITLE / BODY placeholders in the same\n" +
			"atomic batchUpdate that creates the slide, so one call leaves a populated\n" +
			"slide rather than empty placeholders. They assume a layout that actually\n" +
			"has those placeholders (e.g. TITLE_AND_BODY); against BLANK they have\n" +
			"nothing to fill. --at is a 0-based insertion index — omit it to append.",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			pid := extractPresentationID(args[0])
			slideID := s.newObjectID("s_")
			createSlide := map[string]any{
				"objectId":             slideID,
				"slideLayoutReference": map[string]any{"predefinedLayout": layout},
			}
			if cmd.Flags().Changed("at") {
				createSlide["insertionIndex"] = at
			}
			var mappings []any
			var textReqs []any
			if title != "" {
				titleID := s.newObjectID("t_")
				mappings = append(mappings, placeholderMapping(titleID, "TITLE"))
				textReqs = append(textReqs, map[string]any{"insertText": map[string]any{"objectId": titleID, "text": title}})
			}
			if body != "" {
				bodyID := s.newObjectID("b_")
				mappings = append(mappings, placeholderMapping(bodyID, "BODY"))
				textReqs = append(textReqs, map[string]any{"insertText": map[string]any{"objectId": bodyID, "text": body}})
			}
			if len(mappings) > 0 {
				createSlide["placeholderIdMappings"] = mappings
			}
			requests := append([]any{map[string]any{"createSlide": createSlide}}, textReqs...)
			respBody, err := s.batchUpdate(cmd.Context(), token, pid, requests)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(respBody)
			}
			fmt.Fprintf(s.stdout(), "added slide %s\n", slideID)
			return nil
		},
	}
	cmd.Flags().StringVar(&layout, "layout", "TITLE_AND_BODY", "PredefinedLayout (BLANK, TITLE, TITLE_AND_BODY, SECTION_HEADER, ...)")
	cmd.Flags().StringVar(&title, "title", "", "text for the slide's TITLE placeholder")
	cmd.Flags().StringVar(&body, "body", "", "text for the slide's BODY placeholder")
	cmd.Flags().IntVar(&at, "at", 0, "0-based insertion index (default: append to the end)")
	return cmd
}

func placeholderMapping(objectID, placeholderType string) map[string]any {
	return map[string]any{
		"objectId":          objectID,
		"layoutPlaceholder": map[string]any{"type": placeholderType},
	}
}

// newSlidesDuplicateCmd duplicates a slide and, if --at is given, moves the
// copy — merged into one batchUpdate by assigning the copy a known id.
func (s *Service) newSlidesDuplicateCmd(token string) *cobra.Command {
	var at int
	cmd := &cobra.Command{
		Use:   "duplicate <presentation-id-or-url> <slide-id>",
		Short: "Duplicate a slide (optionally positioning the copy with --at)",
		Long: "Copies the slide and everything on it. Every element in the copy gets a NEW\n" +
			"object id, so the source slide's element ids do not address the copy — run\n" +
			"`pages get` on the printed new slide id before editing it. Without --at the\n" +
			"copy lands directly after the source; --at is a 0-based index into the deck.\n" +
			"This is the closest thing to templating available here: duplicate a slide\n" +
			"already in the deck, then fill it with `text replace --slide`.",
		Args:        cobra.ExactArgs(2),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			pid := extractPresentationID(args[0])
			srcID := args[1]
			newID := s.newObjectID("s_")
			requests := []any{map[string]any{"duplicateObject": map[string]any{
				"objectId":  srcID,
				"objectIds": map[string]any{srcID: newID},
			}}}
			if cmd.Flags().Changed("at") {
				requests = append(requests, map[string]any{"updateSlidesPosition": map[string]any{
					"slideObjectIds": []string{newID},
					"insertionIndex": at,
				}})
			}
			respBody, err := s.batchUpdate(cmd.Context(), token, pid, requests)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(respBody)
			}
			fmt.Fprintf(s.stdout(), "duplicated %s -> %s\n", srcID, newID)
			return nil
		},
	}
	cmd.Flags().IntVar(&at, "at", 0, "0-based insertion index for the copy (default: right after the source)")
	return cmd
}

// newSlidesMoveCmd reorders one or more slides to a new position.
func (s *Service) newSlidesMoveCmd(token string) *cobra.Command {
	var to int
	cmd := &cobra.Command{
		Use:   "move <presentation-id-or-url> <slide-id>...",
		Short: "Reorder slides to a new position (--to is the 0-based target index)",
		Long: "--to is required and is a 0-based index into the slide order as it stands\n" +
			"BEFORE the move, not after, so moving a slide later in the deck lands\n" +
			"earlier than the raw number suggests. Several slide ids may be given in one\n" +
			"call; they move as a contiguous block and keep their relative order.\n" +
			"Reordering only — nothing is copied or destroyed, and re-reading\n" +
			"`presentations get` afterwards confirms the resulting order.",
		Args:        cobra.MinimumNArgs(2),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("to") {
				return fmt.Errorf("slides: --to is required")
			}
			pid := extractPresentationID(args[0])
			ids := cleanIDs(args[1:])
			if len(ids) == 0 {
				return fmt.Errorf("slides: no slide ids given")
			}
			requests := []any{map[string]any{"updateSlidesPosition": map[string]any{
				"slideObjectIds": ids,
				"insertionIndex": to,
			}}}
			respBody, err := s.batchUpdate(cmd.Context(), token, pid, requests)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(respBody)
			}
			fmt.Fprintf(s.stdout(), "moved %d slide(s) to index %d\n", len(ids), to)
			return nil
		},
	}
	cmd.Flags().IntVar(&to, "to", 0, "0-based target index (required)")
	return cmd
}

// newSlidesDeleteCmd deletes whole slides. This is the highest-risk layer:
// the API has no undo — the soft guardrail (confirm before overwriting/deleting
// a deck the assistant did not create) lives in the skill, not here.
func (s *Service) newSlidesDeleteCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <presentation-id-or-url> <slide-id>...",
		Short: "Delete whole slides (no undo — confirm with the user first for decks you did not create)",
		Long: "Takes slide object ids read from `presentations get`, several per call, and\n" +
			"removes each slide with everything on it. There is no undo and no trash: the\n" +
			"only recovery is the deck owner opening version history in the browser.\n" +
			"Deleting all of a deck's slides leaves an empty presentation rather than\n" +
			"removing the deck, which cannot be deleted on this scope at all.",
		Args:        cobra.MinimumNArgs(2),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			pid := extractPresentationID(args[0])
			ids := cleanIDs(args[1:])
			if len(ids) == 0 {
				return fmt.Errorf("slides: no slide ids given")
			}
			requests := make([]any, 0, len(ids))
			for _, id := range ids {
				requests = append(requests, map[string]any{"deleteObject": map[string]any{"objectId": id}})
			}
			respBody, err := s.batchUpdate(cmd.Context(), token, pid, requests)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(respBody)
			}
			fmt.Fprintf(s.stdout(), "deleted %d slide(s)\n", len(ids))
			return nil
		},
	}
	return cmd
}

// cleanIDs splits every multi-id arg on whitespace and drops empties, killing
// the trailing-space / \r-from-pipeline / several-ids-in-one-arg classes that
// the API rejects as INVALID_ARGUMENT.
func cleanIDs(args []string) []string {
	ids := make([]string, 0, len(args))
	for _, arg := range args {
		ids = append(ids, strings.Fields(arg)...)
	}
	return ids
}
