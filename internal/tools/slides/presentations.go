package slides

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/spf13/cobra"
)

func (s *Service) newPresentationsGetCmd(token string) *cobra.Command {
	var slide string
	cmd := &cobra.Command{
		Use:   "get <presentation-id-or-url>",
		Short: "Read a deck: human outline (object ids + layout + text + notes) by default, raw JSON with --json",
		Long: "The id-discovery call every edit depends on: per slide it prints the slide\n" +
			"object id, the resolved layout name, one line per text-bearing element\n" +
			"(object id, placeholder type, collapsed text) and the speaker notes.\n" +
			"Elements with no text — images, empty shapes — are omitted, so reach for\n" +
			"`pages get` when the target is one of those. --slide narrows the outline to\n" +
			"a single slide by 1-based index or by slide object id. --json returns the\n" +
			"raw Presentation resource, which is very large and rarely worth reading.",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			pid := extractPresentationID(args[0])
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/presentations/"+pid, nil, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var p presentation
			if err := json.Unmarshal(body, &p); err != nil {
				return fmt.Errorf("slides: decode presentation: %w", err)
			}
			filter, err := parseSlideFilter(slide)
			if err != nil {
				return err
			}
			writeOutline(s.stdout(), &p, filter)
			return nil
		},
	}
	cmd.Flags().StringVar(&slide, "slide", "", "limit the outline to one slide: a 1-based index (N) or a slide object id")
	return cmd
}

// parseSlideFilter reads the --slide value as a 1-based index or an object id.
func parseSlideFilter(slide string) (slideFilter, error) {
	if slide == "" {
		return slideFilter{all: true}, nil
	}
	if n, err := strconv.Atoi(slide); err == nil {
		if n < 1 {
			return slideFilter{}, fmt.Errorf("slides: --slide index must be >= 1, got %d", n)
		}
		return slideFilter{index: n}, nil
	}
	return slideFilter{objectID: slide}, nil
}

func (s *Service) newPresentationsCreateCmd(token string) *cobra.Command {
	var title string
	cmd := &cobra.Command{
		Use:   "create --title <title>",
		Short: "Create a new empty presentation; returns its id and editor URL",
		Long: "--title is required and is the only property settable at creation; the deck\n" +
			"lands in the connected account's Drive, owned by that account, visible to\n" +
			"nobody else until its owner shares it — which cannot be done from here.\n" +
			"Populate it with `slides add`. Starting from an existing template is not\n" +
			"possible on this scope: copying a deck needs Drive, so either rebuild the\n" +
			"template's slides here or have its owner copy it and hand over the new link.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if title == "" {
				return fmt.Errorf("slides: --title is required")
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/presentations", nil, map[string]any{"title": title})
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var p presentation
			if err := json.Unmarshal(body, &p); err != nil {
				return fmt.Errorf("slides: decode created presentation: %w", err)
			}
			fmt.Fprintf(s.stdout(), "created presentation %s\nURL: %s\n", p.PresentationID, presentationURL(p.PresentationID))
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "presentation title (required)")
	return cmd
}
