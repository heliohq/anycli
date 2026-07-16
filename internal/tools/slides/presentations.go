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
		Args:  cobra.ExactArgs(1),
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
		Args:  cobra.NoArgs,
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
