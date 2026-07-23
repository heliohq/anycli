package dropboxsign

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newTemplateListCmd lists the reusable templates the account can send with.
func (s *Service) newTemplateListCmd(token string) *cobra.Command {
	var (
		page     int
		pageSize int
		query    string
	)
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List reusable templates",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if page > 0 {
				q.Set("page", strconv.Itoa(page))
			}
			if pageSize > 0 {
				q.Set("page_size", strconv.Itoa(pageSize))
			}
			if query != "" {
				q.Set("query", query)
			}
			body, err := s.callGET(cmd.Context(), token, "/template/list", q)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().IntVar(&page, "page", 0, "page number (1-based)")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "results per page")
	cmd.Flags().StringVar(&query, "query", "", "filter query (Dropbox Sign search syntax)")
	return cmd
}

// newTemplateGetCmd fetches one template's roles and fields.
func (s *Service) newTemplateGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <template_id>",
		Short:       "Get one template (roles and fields)",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.callGET(cmd.Context(), token, "/template/"+url.PathEscape(args[0]), nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// newAccountGetCmd returns the authenticated account's identity and quota. It
// is also the provider identity endpoint (the bearer token identifies the
// account, so no query params are needed).
func (s *Service) newAccountGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "get",
		Short:       "Get the authenticated account (identity and quota)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.callGET(cmd.Context(), token, "/account", nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}
