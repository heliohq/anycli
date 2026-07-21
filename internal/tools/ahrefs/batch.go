package ahrefs

import (
	"net/http"
	"strings"

	"github.com/spf13/cobra"
)

const batchDefaultSelect = "url,domain_rating,backlinks,refdomains,org_traffic,org_keywords"

// batchTarget is one entry in the batch-analysis request. The API requires url,
// mode, and protocol on every target.
type batchTarget struct {
	URL      string `json:"url"`
	Mode     string `json:"mode"`
	Protocol string `json:"protocol"`
}

// batchRequest is the POST /batch-analysis/batch-analysis body.
type batchRequest struct {
	Select  []string      `json:"select"`
	Targets []batchTarget `json:"targets"`
	Country string        `json:"country,omitempty"`
	OrderBy []string      `json:"order_by,omitempty"`
}

// newBatchCmd wraps POST /batch-analysis/batch-analysis: up to 100 targets in a
// single request — the unit-efficient "compare these domains" path. --mode and
// --protocol apply to every target (the API requires both per target); their
// defaults match Ahrefs' documented defaults (subdomains / both).
func (s *Service) newBatchCmd(token string) *cobra.Command {
	var targets, selectFields, country, mode, protocol string
	cmd := &cobra.Command{
		Use:   "batch",
		Short: "Compare up to 100 targets in one request (POST /batch-analysis)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			urls := splitCSV(targets)
			if len(urls) == 0 {
				return &usageError{msg: "ahrefs: --targets is required (comma-separated)"}
			}
			fields := splitCSV(selectFields)
			if len(fields) == 0 {
				return &usageError{msg: "ahrefs: --select must not be empty"}
			}
			body := batchRequest{Select: fields, Country: country}
			for _, u := range urls {
				body.Targets = append(body.Targets, batchTarget{URL: u, Mode: mode, Protocol: protocol})
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/batch-analysis/batch-analysis", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&targets, "targets", "", "comma-separated domains/URLs, up to 100 (required)")
	cmd.Flags().StringVar(&selectFields, "select", batchDefaultSelect, "comma-separated fields to return (unit cost scales with fields)")
	cmd.Flags().StringVar(&country, "country", "", "ISO country code for traffic/keyword figures (optional)")
	cmd.Flags().StringVar(&mode, "mode", "subdomains", "target mode for every target: exact|prefix|domain|subdomains")
	cmd.Flags().StringVar(&protocol, "protocol", "both", "protocol for every target: both|http|https")
	return cmd
}

// splitCSV splits a comma-separated flag value, trimming spaces and dropping
// empty entries.
func splitCSV(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
