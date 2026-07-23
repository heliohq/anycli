package customerio

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newTransactionalListCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:         "list",
		Short:       "List transactional message templates (GET /v1/transactional)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd, key, http.MethodGet, "/v1/transactional", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newTransactionalGetCmd(key string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get a transactional template (GET /v1/transactional/{id})",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd, key, http.MethodGet, "/v1/transactional/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "transactional message id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newTransactionalMetricsCmd(key string) *cobra.Command {
	var id string
	var m metricsParams
	cmd := &cobra.Command{
		Use:         "metrics",
		Short:       "Transactional template performance metrics (GET /v1/transactional/{id}/metrics)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			m.apply(q)
			resp, err := s.call(cmd, key, http.MethodGet, "/v1/transactional/"+url.PathEscape(id)+"/metrics", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "transactional message id")
	registerMetricsFlags(cmd, &m)
	_ = cmd.MarkFlagRequired("id")
	return cmd
}
