package customerio

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newNewsletterListCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List newsletters (GET /v1/newsletters)",
		Long: "One unpaginated response, no filter flags. Newsletters are one-off sends,\n" +
			"distinct from the ongoing automations `campaign list` returns and from the\n" +
			"API-triggered `broadcast list`; the three are separate id spaces and an id\n" +
			"from one will not resolve in another.",
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
		Use:   "get",
		Short: "Get a newsletter (GET /v1/newsletters/{id})",
		Long: "The newsletter's configuration and content metadata rather than how it\n" +
			"performed, which is `newsletter metrics`. Its id is also what `export\n" +
			"deliveries --newsletter` takes.",
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
		Use:   "metrics",
		Short: "Newsletter performance metrics (GET /v1/newsletters/{id}/metrics or /metrics/links)",
		Long: "--links switches to per-URL click breakdown; without it the report is the\n" +
			"delivery funnel. --period (hours, days, weeks or months) and --steps set\n" +
			"the window, and leaving both unset accepts Customer.io's default rather\n" +
			"than the newsletter's whole lifetime. There is no journey report here —\n" +
			"that concept only exists for campaigns.",
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
