package slides

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// newImagesInsertCmd inserts an image from a public URL (Google fetches it, so
// it must be publicly reachable: ≤50MB / ≤25MP / PNG·JPEG·GIF). Local files
// need a Drive upload surface, which v1 does not have (design 303).
func (s *Service) newImagesInsertCmd(token string) *cobra.Command {
	var slideID, imageURL, at, size string
	cmd := &cobra.Command{
		Use:         "insert <presentation-id-or-url> --slide <slide-id> --url <https-url>",
		Short:       "Insert an image from a public URL onto a slide (--at x,y and --size WxH in points)",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if slideID == "" {
				return fmt.Errorf("slides: --slide is required")
			}
			if imageURL == "" {
				return fmt.Errorf("slides: --url is required")
			}
			pid := extractPresentationID(args[0])
			imageID := s.newObjectID("img_")
			elementProps := map[string]any{"pageObjectId": slideID}
			if size != "" {
				sz, err := parseSize(size)
				if err != nil {
					return err
				}
				elementProps["size"] = sz
			}
			if at != "" {
				tf, err := parseTransform(at)
				if err != nil {
					return err
				}
				elementProps["transform"] = tf
			}
			requests := []any{map[string]any{"createImage": map[string]any{
				"objectId":          imageID,
				"url":               imageURL,
				"elementProperties": elementProps,
			}}}
			respBody, err := s.batchUpdate(cmd.Context(), token, pid, requests)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(respBody)
			}
			fmt.Fprintf(s.stdout(), "inserted image %s on slide %s\n", imageID, slideID)
			return nil
		},
	}
	cmd.Flags().StringVar(&slideID, "slide", "", "target slide object id (required)")
	cmd.Flags().StringVar(&imageURL, "url", "", "public https URL of the image (required)")
	cmd.Flags().StringVar(&at, "at", "", "top-left position in points, as x,y")
	cmd.Flags().StringVar(&size, "size", "", "image size in points, as WxH")
	return cmd
}

// parseSize turns "WxH" (points) into a Slides Size object.
func parseSize(spec string) (map[string]any, error) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(spec)), "x")
	if len(parts) != 2 {
		return nil, fmt.Errorf("slides: --size must look like WxH, got %q", spec)
	}
	w, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil || w <= 0 {
		return nil, fmt.Errorf("slides: --size width must be a positive number, got %q", parts[0])
	}
	h, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil || h <= 0 {
		return nil, fmt.Errorf("slides: --size height must be a positive number, got %q", parts[1])
	}
	return map[string]any{
		"width":  map[string]any{"magnitude": w, "unit": "PT"},
		"height": map[string]any{"magnitude": h, "unit": "PT"},
	}, nil
}

// parseTransform turns "x,y" (points) into an affine transform that positions
// the element's top-left corner with no scaling.
func parseTransform(spec string) (map[string]any, error) {
	parts := strings.Split(strings.TrimSpace(spec), ",")
	if len(parts) != 2 {
		return nil, fmt.Errorf("slides: --at must look like x,y, got %q", spec)
	}
	x, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return nil, fmt.Errorf("slides: --at x must be a number, got %q", parts[0])
	}
	y, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return nil, fmt.Errorf("slides: --at y must be a number, got %q", parts[1])
	}
	return map[string]any{
		"scaleX":     1,
		"scaleY":     1,
		"translateX": x,
		"translateY": y,
		"unit":       "PT",
	}, nil
}

// newElementsDeleteCmd deletes page elements (shapes, images, ...) inside a
// slide. Highest-risk layer — no undo; the confirm-first guardrail lives in
// the skill.
func (s *Service) newElementsDeleteCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "delete <presentation-id-or-url> <element-id>...",
		Short:       "Delete page elements inside slides (no undo — confirm first for content you did not create)",
		Args:        cobra.MinimumNArgs(2),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			pid := extractPresentationID(args[0])
			ids := cleanIDs(args[1:])
			if len(ids) == 0 {
				return fmt.Errorf("slides: no element ids given")
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
			fmt.Fprintf(s.stdout(), "deleted %d element(s)\n", len(ids))
			return nil
		},
	}
	return cmd
}
