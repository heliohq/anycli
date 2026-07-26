package customerio

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newTransactionalListCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List transactional message templates (GET /v1/transactional)",
		Long: "One unpaginated response. These are the templates `send email\n" +
			"--transactional-id` addresses, so this is the prerequisite for sending\n" +
			"anything: a transactional message must already exist in the workspace and\n" +
			"cannot be created from here.",
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
		Use:   "get",
		Short: "Get a transactional template (GET /v1/transactional/{id})",
		Long: "Reads the template's configuration — subject, from address and body —\n" +
			"which is what `send email` uses unless the corresponding override flag is\n" +
			"passed. Reading it first is how to tell whether an override is needed at\n" +
			"all.",
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
		Use:   "metrics",
		Short: "Transactional template performance metrics (GET /v1/transactional/{id}/metrics)",
		Long: "Aggregate delivery metrics for everything sent through this template.\n" +
			"--period (hours, days, weeks or months) with --steps sets the window;\n" +
			"unset takes the provider default rather than all history. Tracing one\n" +
			"particular send instead means `message list` or `person messages`.",
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
