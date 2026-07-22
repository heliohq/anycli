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
		Args:  cobra.NoArgs,
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
		Args:  cobra.ExactArgs(1),
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
		Args:  cobra.NoArgs,
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
		Args:  cobra.ExactArgs(1),
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
