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
		Use:   "list",
		Short: "List boards (GET /boards)",
		Long: "The boards the connected account owns, one cursor page at a time via\n" +
			"`--page-size` and `--bookmark`. Board ids come from here and nowhere else:\n" +
			"there is no lookup by board name or URL, so resolving \"the Recipes board\"\n" +
			"means paging this list and matching `name` locally. Those ids are what\n" +
			"`board pins`, `board sections`, `board add-section` and\n" +
			"`pin create --board-id` all take.",
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
		Use:   "get <board_id>",
		Short: "Get one board (GET /boards/{board_id})",
		Long: "Takes the board id from `board list` — a board name or a pinterest.com URL\n" +
			"will not resolve. Returns the board's `name`, `description`, `privacy` and\n" +
			"pin count, but not its pins; those are `board pins <board_id>`.",
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
		Use:   "create",
		Short: "Create a board (POST /boards)",
		Long: "`--name` is required. `--privacy` is PUBLIC, PROTECTED or SECRET and is\n" +
			"omitted from the request when unset, so Pinterest's own default applies;\n" +
			"the value is passed through unvalidated, so a misspelt level surfaces as a\n" +
			"provider 400 rather than a usage error. The response carries the new\n" +
			"board's `id`, which is what `pin create --board-id` needs. Privacy cannot\n" +
			"be changed afterwards from this tool — there is no board update verb.",
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
		Use:   "delete <board_id>",
		Short: "Delete a board (DELETE /boards/{board_id})",
		Long: "Irreversible, and it takes the board's pins down with it — there is no\n" +
			"archive or restore. Pinterest answers 204 with an empty body, so a\n" +
			"`{\"deleted\":true}` receipt is printed instead, because bare empty output\n" +
			"cannot be told apart from a call that did nothing.",
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
		Use:   "sections <board_id>",
		Short: "List a board's sections (GET /boards/{board_id}/sections)",
		Long: "Sections are sub-groupings inside one board, so a board with none returns an\n" +
			"empty page rather than an error. Cursor-paged with `--page-size` and\n" +
			"`--bookmark`. The section ids here are what `pin create --section-id`\n" +
			"takes; a pin created without one lands loose on the board.",
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
		Use:   "add-section <board_id>",
		Short: "Create a board section (POST /boards/{board_id}/sections)",
		Long: "`--name` is required and the argument is the board the section belongs to.\n" +
			"The response carries the new section's `id` for `pin create --section-id`.\n" +
			"Sections cannot be renamed, emptied or removed through this tool, so a\n" +
			"mistaken one stays on the board.",
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
		Use:   "pins <board_id>",
		Short: "List pins on a board (GET /boards/{board_id}/pins)",
		Long: "Scoped to one board, unlike `pin list`, which spans every board on the\n" +
			"account — reaching for this first is much cheaper than paging the whole\n" +
			"account and filtering on `board_id`. Cursor-paged with `--page-size` and\n" +
			"`--bookmark`. Pins in the board's sections are included; there is no\n" +
			"section-scoped listing.",
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
