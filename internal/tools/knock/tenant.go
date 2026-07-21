package knock

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newTenantCmd groups the tenant verbs. Tenants scope notifications by
// customer/workspace so per-tenant branding and preferences apply.
func (s *Service) newTenantCmd(key string) *cobra.Command {
	group := newGroupCmd("tenant", "Manage tenants (customer/workspace scopes)")
	group.AddCommand(
		s.newTenantSetCmd(key),
		s.newTenantGetCmd(key),
		s.newTenantListCmd(key),
		s.newTenantDeleteCmd(key),
	)
	return group
}

func (s *Service) newTenantSetCmd(key string) *cobra.Command {
	var (
		id   string
		data string
	)
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set (create or update) a tenant",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID("id", id); err != nil {
				return err
			}
			body := map[string]any{}
			if data != "" {
				parsed, decErr := decodeJSONFlag("data", data)
				if decErr != nil {
					return decErr
				}
				obj, ok := parsed.(map[string]any)
				if !ok {
					return &usageError{msg: "--data must be a JSON object"}
				}
				body = obj
			}
			return s.callEmit(cmd.Context(), key, http.MethodPut, "/tenants/"+url.PathEscape(id), nil, body, nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "tenant id (required)")
	cmd.Flags().StringVar(&data, "data", "", "tenant properties as a JSON object (settings, branding, …)")
	return cmd
}

func (s *Service) newTenantGetCmd(key string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a tenant",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID("id", id); err != nil {
				return err
			}
			return s.callEmit(cmd.Context(), key, http.MethodGet, "/tenants/"+url.PathEscape(id), nil, nil, nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "tenant id (required)")
	return cmd
}

func (s *Service) newTenantListCmd(key string) *cobra.Command {
	var (
		pageSize int
		after    string
		before   string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tenants",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			addPaging(q, pageSize, after, before)
			return s.callEmit(cmd.Context(), key, http.MethodGet, "/tenants", q, nil, nil)
		},
	}
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "page size (Knock default 50)")
	cmd.Flags().StringVar(&after, "after", "", "pagination cursor (next page)")
	cmd.Flags().StringVar(&before, "before", "", "pagination cursor (previous page)")
	return cmd
}

func (s *Service) newTenantDeleteCmd(key string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a tenant",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID("id", id); err != nil {
				return err
			}
			return s.callEmit(cmd.Context(), key, http.MethodDelete, "/tenants/"+url.PathEscape(id), nil, nil, nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "tenant id (required)")
	return cmd
}
