package savvycal

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newLinkCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "link",
		Short: "Scheduling links (list, get, create, update, toggle, duplicate, delete, slots)",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		s.newLinkListCmd(token),
		s.newLinkGetCmd(token),
		s.newLinkCreateCmd(token),
		s.newLinkUpdateCmd(token),
		s.newLinkToggleCmd(token),
		s.newLinkDuplicateCmd(token),
		s.newLinkDeleteCmd(token),
		s.newLinkSlotsCmd(token),
	)
	return cmd
}

func (s *Service) newLinkListCmd(token string) *cobra.Command {
	var after, before string
	var limit int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List your scheduling links (GET /v1/links)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if cmd.Flags().Changed("limit") {
				q.Set("limit", itoa(limit))
			}
			if after != "" {
				q.Set("after", after)
			}
			if before != "" {
				q.Set("before", before)
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/links", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "page size (max 100)")
	cmd.Flags().StringVar(&after, "after", "", "pagination cursor (next page)")
	cmd.Flags().StringVar(&before, "before", "", "pagination cursor (previous page)")
	return cmd
}

func (s *Service) newLinkGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <link_id>",
		Short:       "Get a scheduling link (GET /v1/links/:link_id)",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/links/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newLinkCreateCmd(token string) *cobra.Command {
	var name, privateName, description, linkType, scope string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a scheduling link (POST /v1/links, or /v1/scopes/:scope/links with --scope)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{"name": name}
			if privateName != "" {
				body["private_name"] = privateName
			}
			if description != "" {
				body["description"] = description
			}
			if linkType != "" {
				body["type"] = linkType
			}
			path := "/links"
			if scope != "" {
				path = "/scopes/" + url.PathEscape(scope) + "/links"
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, path, nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "public link name")
	cmd.Flags().StringVar(&privateName, "private-name", "", "internal-only name")
	cmd.Flags().StringVar(&description, "description", "", "link description")
	cmd.Flags().StringVar(&linkType, "type", "", "recurring (multi-use, default) | single (single-use)")
	cmd.Flags().StringVar(&scope, "scope", "", "team/individual scope slug (omit for personal scope)")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func (s *Service) newLinkUpdateCmd(token string) *cobra.Command {
	var name, privateName, description, linkType string
	cmd := &cobra.Command{
		Use:         "update <link_id>",
		Short:       "Update a scheduling link (PATCH /v1/links/:link_id)",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{}
			if cmd.Flags().Changed("name") {
				body["name"] = name
			}
			if cmd.Flags().Changed("private-name") {
				body["private_name"] = privateName
			}
			if cmd.Flags().Changed("description") {
				body["description"] = description
			}
			if cmd.Flags().Changed("type") {
				body["type"] = linkType
			}
			if len(body) == 0 {
				return &usageError{msg: "link update requires at least one of --name, --private-name, --description, --type"}
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPatch, "/links/"+url.PathEscape(args[0]), nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "public link name")
	cmd.Flags().StringVar(&privateName, "private-name", "", "internal-only name")
	cmd.Flags().StringVar(&description, "description", "", "link description")
	cmd.Flags().StringVar(&linkType, "type", "", "recurring | single")
	return cmd
}

func (s *Service) newLinkToggleCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "toggle <link_id>",
		Short:       "Toggle a link between active and disabled (POST /v1/links/:link_id/toggle)",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/links/"+url.PathEscape(args[0])+"/toggle", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newLinkDuplicateCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "duplicate <link_id>",
		Short:       "Duplicate a scheduling link (POST /v1/links/:link_id/duplicate)",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/links/"+url.PathEscape(args[0])+"/duplicate", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newLinkDeleteCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "delete <link_id>",
		Short:       "Delete a scheduling link (DELETE /v1/links/:link_id)",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodDelete, "/links/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newLinkSlotsCmd(token string) *cobra.Command {
	var from, until string
	cmd := &cobra.Command{
		Use:   "slots <link_id>",
		Short: "Get available time slots for a link (GET /v1/links/:link_id/slots)",
		Long: "Get available booking slots. Each slot carries a cumulative `rank`; " +
			"to offer non-overlapping options filter to a single rank (rank === N), not rank <= N.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			if from != "" {
				q.Set("from", from)
			}
			if until != "" {
				q.Set("until", until)
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/links/"+url.PathEscape(args[0])+"/slots", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "ISO-8601 window start (default now)")
	cmd.Flags().StringVar(&until, "until", "", "ISO-8601 window end (default +7d)")
	return cmd
}
