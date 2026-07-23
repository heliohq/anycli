package customerio

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newNewsletterListCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:         "list",
		Short:       "List newsletters (GET /v1/newsletters)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd, key, http.MethodGet, "/v1/newsletters", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newNewsletterGetCmd(key string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get a newsletter (GET /v1/newsletters/{id})",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd, key, http.MethodGet, "/v1/newsletters/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "newsletter id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newNewsletterMetricsCmd(key string) *cobra.Command {
	var id string
	var links bool
	var m metricsParams
	cmd := &cobra.Command{
		Use:         "metrics",
		Short:       "Newsletter performance metrics (GET /v1/newsletters/{id}/metrics or /metrics/links)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			m.apply(q)
			path := "/v1/newsletters/" + url.PathEscape(id) + "/metrics"
			if links {
				path += "/links"
			}
			resp, err := s.call(cmd, key, http.MethodGet, path, q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "newsletter id")
	cmd.Flags().BoolVar(&links, "links", false, "report per-link click metrics (/metrics/links)")
	registerMetricsFlags(cmd, &m)
	_ = cmd.MarkFlagRequired("id")
	return cmd
}
