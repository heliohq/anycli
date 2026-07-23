package mailerlite

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newGroupCmd builds the `mailerlite group` command tree — segmentation by
// group, including the everyday assign/unassign of a subscriber to a group.
func (s *Service) newGroupCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "group", Short: "Groups (list, create, update, delete, subscribers, assign, unassign)"}
	cmd.AddCommand(
		s.newGroupListCmd(token),
		s.newGroupCreateCmd(token),
		s.newGroupUpdateCmd(token),
		s.newGroupDeleteCmd(token),
		s.newGroupSubscribersCmd(token),
		s.newGroupAssignCmd(token),
		s.newGroupUnassignCmd(token),
	)
	return cmd
}

func (s *Service) newGroupListCmd(token string) *cobra.Command {
	var name string
	var limit, page int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List groups (GET /groups)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if name != "" {
				q.Set("filter[name]", name)
			}
			setLimitPage(cmd, q, limit, page)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/groups", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "filter by group name")
	cmd.Flags().IntVar(&limit, "limit", 25, "page size (default 25)")
	cmd.Flags().IntVar(&page, "page", 1, "page number (starts at 1)")
	return cmd
}

func (s *Service) newGroupCreateCmd(token string) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a group (POST /groups)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/groups", nil, map[string]any{"name": name})
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "group name (required)")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func (s *Service) newGroupUpdateCmd(token string) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Rename a group (PUT /groups/{id})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodPut, "/groups/"+url.PathEscape(args[0]), nil, map[string]any{"name": name})
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "new group name (required)")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func (s *Service) newGroupDeleteCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a group (DELETE /groups/{id})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodDelete, "/groups/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newGroupSubscribersCmd(token string) *cobra.Command {
	var status, cursor string
	var limit int
	cmd := &cobra.Command{
		Use:   "subscribers <id>",
		Short: "List a group's subscribers (GET /groups/{id}/subscribers)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			if status != "" {
				q.Set("filter[status]", status)
			}
			setLimitCursor(cmd, q, limit, cursor)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/groups/"+url.PathEscape(args[0])+"/subscribers", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "filter by status: active|unsubscribed|unconfirmed|bounced|junk")
	cmd.Flags().IntVar(&limit, "limit", 25, "page size (default 25)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor")
	return cmd
}

func (s *Service) newGroupAssignCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "assign <subscriber-id> <group-id>",
		Short: "Assign a subscriber to a group (POST /subscribers/{sub}/groups/{group})",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "/subscribers/" + url.PathEscape(args[0]) + "/groups/" + url.PathEscape(args[1])
			resp, err := s.call(cmd.Context(), token, http.MethodPost, path, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newGroupUnassignCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "unassign <subscriber-id> <group-id>",
		Short: "Unassign a subscriber from a group (DELETE /subscribers/{sub}/groups/{group})",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "/subscribers/" + url.PathEscape(args[0]) + "/groups/" + url.PathEscape(args[1])
			resp, err := s.call(cmd.Context(), token, http.MethodDelete, path, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}
