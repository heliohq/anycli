package pinterest

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newPinCmd(token string) *cobra.Command {
	cmd := newGroupCmd("pin", "Manage pins")
	cmd.AddCommand(
		s.newPinListCmd(token),
		s.newPinGetCmd(token),
		s.newPinCreateCmd(token),
		s.newPinDeleteCmd(token),
	)
	return cmd
}

func (s *Service) newPinListCmd(token string) *cobra.Command {
	var page pageParams
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List pins on the account (GET /pins)",
		Long: "Spans every board on the account, so narrowing to one board with\n" +
			"`board pins <board_id>` is the cheaper read whenever the board is known.\n" +
			"There is no search or filter — not by title, link, board or date — so the\n" +
			"`--page-size` / `--bookmark` cursor plus local matching is the only way to\n" +
			"find a particular pin.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			page.apply(q)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/pins", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerPageFlags(cmd, &page)
	return cmd
}

func (s *Service) newPinGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <pin_id>",
		Short: "Get one pin (GET /pins/{pin_id})",
		Long: "Returns the pin with the `board_id` and `board_section_id` it sits in, its\n" +
			"`link`, `title`, `description` and media. Pins are immutable here: there is\n" +
			"no update verb, so correcting a title, link or board means `pin delete`\n" +
			"followed by a fresh `pin create`, which produces a different pin id.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/pins/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newPinCreateCmd(token string) *cobra.Command {
	var boardID, imageURL, title, description, link, sectionID string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an image pin (POST /pins)",
		Long: "`--board-id` and `--image-url` are both required — every pin belongs to a\n" +
			"board, and the image is one Pinterest FETCHES from a publicly reachable\n" +
			"URL, not a local file upload. An URL Pinterest cannot reach, or that is not\n" +
			"really an image, comes back as a provider 400 about a valid image url.\n" +
			"Image pins only: video and carousel pins have no command here.\n" +
			"\n" +
			"`--section-id` places the pin in a section from `board sections`;\n" +
			"`--link` is the destination a click opens, and without it the pin leads\n" +
			"nowhere. The pin is live on the account the moment the call returns.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if boardID == "" {
				return &usageError{msg: "pinterest: --board-id is required"}
			}
			if imageURL == "" {
				return &usageError{msg: "pinterest: --image-url is required"}
			}
			body := map[string]any{
				"board_id": boardID,
				"media_source": map[string]any{
					"source_type": "image_url",
					"url":         imageURL,
				},
			}
			if title != "" {
				body["title"] = title
			}
			if description != "" {
				body["description"] = description
			}
			if link != "" {
				body["link"] = link
			}
			if sectionID != "" {
				body["board_section_id"] = sectionID
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/pins", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&boardID, "board-id", "", "target board id (required)")
	cmd.Flags().StringVar(&imageURL, "image-url", "", "publicly reachable source image URL (required)")
	cmd.Flags().StringVar(&title, "title", "", "pin title")
	cmd.Flags().StringVar(&description, "description", "", "pin description")
	cmd.Flags().StringVar(&link, "link", "", "destination link for the pin")
	cmd.Flags().StringVar(&sectionID, "section-id", "", "board section id to place the pin in")
	return cmd
}

func (s *Service) newPinDeleteCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <pin_id>",
		Short: "Delete a pin (DELETE /pins/{pin_id})",
		Long: "Irreversible: the pin cannot be restored, and re-creating it makes a new pin\n" +
			"that starts over with no saves or impressions. Pinterest answers 204 with\n" +
			"an empty body, so a `{\"deleted\":true}` receipt is printed instead of\n" +
			"nothing.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodDelete, "/pins/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			if len(resp) == 0 {
				return s.emit([]byte(`{"deleted":true}`))
			}
			return s.emit(resp)
		},
	}
}
