package typeform

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newWorkspaceListCmd is `workspace list` (GET /workspaces): every workspace
// the account can access, so form creation lands in the right place.
// Pagination is surfaced via --page/--page-size (max 200). Output JSON.
func (s *Service) newWorkspaceListCmd(token string) *cobra.Command {
	var search string
	var page, pageSize int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workspaces (GET /workspaces)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if search != "" {
				q.Set("search", search)
			}
			if page > 0 {
				q.Set("page", strconv.Itoa(page))
			}
			if pageSize > 0 {
				q.Set("page_size", strconv.Itoa(pageSize))
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/workspaces", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&search, "search", "", "return workspaces whose name contains this string")
	cmd.Flags().IntVar(&page, "page", 0, "page number (default 1)")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "results per page (default 10, max 200)")
	return cmd
}

// newWorkspaceGetCmd is `workspace get <workspace_id>`
// (GET /workspaces/{id}). Output JSON.
func (s *Service) newWorkspaceGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <workspace_id>",
		Short: "Retrieve a workspace (GET /workspaces/{id})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/workspaces/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	return cmd
}

// newWorkspaceCreateCmd is `workspace create --name <name>`
// (POST /workspaces): body is a single `name` field. The workspace is created
// in the account where the user has the organisation owner role. Output JSON
// (the created workspace).
func (s *Service) newWorkspaceCreateCmd(token string) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a workspace (POST /workspaces)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/workspaces", nil, map[string]any{"name": name})
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "name of the new workspace (required)")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}
