package pinterest

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newBoardCmd(token string) *cobra.Command {
	cmd := newGroupCmd("board", "Manage boards and board sections")
	cmd.AddCommand(
		s.newBoardListCmd(token),
		s.newBoardGetCmd(token),
		s.newBoardCreateCmd(token),
		s.newBoardDeleteCmd(token),
		s.newBoardSectionsCmd(token),
		s.newBoardAddSectionCmd(token),
		s.newBoardPinsCmd(token),
	)
	return cmd
}

func (s *Service) newBoardListCmd(token string) *cobra.Command {
	var page pageParams
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List boards (GET /boards)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			page.apply(q)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/boards", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerPageFlags(cmd, &page)
	return cmd
}

func (s *Service) newBoardGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <board_id>",
		Short:       "Get one board (GET /boards/{board_id})",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/boards/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newBoardCreateCmd(token string) *cobra.Command {
	var name, description, privacy string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a board (POST /boards)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if name == "" {
				return &usageError{msg: "pinterest: --name is required"}
			}
			body := map[string]any{"name": name}
			if description != "" {
				body["description"] = description
			}
			if privacy != "" {
				body["privacy"] = privacy
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/boards", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "board name (required)")
	cmd.Flags().StringVar(&description, "description", "", "board description")
	cmd.Flags().StringVar(&privacy, "privacy", "", "board privacy: PUBLIC|PROTECTED|SECRET (Pinterest default when omitted)")
	return cmd
}

func (s *Service) newBoardDeleteCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "delete <board_id>",
		Short:       "Delete a board (DELETE /boards/{board_id})",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodDelete, "/boards/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			// 204 No Content on delete → emit a small receipt so an agent
			// reading stdout sees a definite success rather than empty output.
			if len(resp) == 0 {
				return s.emit([]byte(`{"deleted":true}`))
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newBoardSectionsCmd(token string) *cobra.Command {
	var page pageParams
	cmd := &cobra.Command{
		Use:         "sections <board_id>",
		Short:       "List a board's sections (GET /boards/{board_id}/sections)",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			page.apply(q)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/boards/"+url.PathEscape(args[0])+"/sections", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerPageFlags(cmd, &page)
	return cmd
}

func (s *Service) newBoardAddSectionCmd(token string) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:         "add-section <board_id>",
		Short:       "Create a board section (POST /boards/{board_id}/sections)",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return &usageError{msg: "pinterest: --name is required"}
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/boards/"+url.PathEscape(args[0])+"/sections", nil, map[string]any{"name": name})
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "section name (required)")
	return cmd
}

func (s *Service) newBoardPinsCmd(token string) *cobra.Command {
	var page pageParams
	cmd := &cobra.Command{
		Use:         "pins <board_id>",
		Short:       "List pins on a board (GET /boards/{board_id}/pins)",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			page.apply(q)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/boards/"+url.PathEscape(args[0])+"/pins", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerPageFlags(cmd, &page)
	return cmd
}
