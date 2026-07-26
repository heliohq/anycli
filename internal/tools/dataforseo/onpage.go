package dataforseo

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newOnpageCmd is the `onpage` resource group.
func (s *Service) newOnpageCmd(credential string) *cobra.Command {
	onpage := newGroupCmd("onpage", "On-page technical checks")
	onpage.AddCommand(s.newOnpageCheckCmd(credential))
	return onpage
}

// newOnpageCheckCmd runs an instant single-page on-page audit.
func (s *Service) newOnpageCheckCmd(credential string) *cobra.Command {
	var url string
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Instant on-page audit of a single URL",
		Long: "Fetches the page live and returns its status, timings, meta tags, headings,\n" +
			"content counts and the issues DataForSEO flags on it. --url must be\n" +
			"absolute, scheme included. This is the instant-page endpoint: it fetches\n" +
			"exactly one URL and follows no links, so auditing a site means calling it\n" +
			"per page — a real crawl is not wrapped here.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			task := map[string]any{"url": url}
			return s.do(cmd.Context(), credential, http.MethodPost, "/on_page/instant_pages", task)
		},
	}
	cmd.Annotations = readOnly
	cmd.Flags().StringVar(&url, "url", "", "absolute URL of the target page (required)")
	_ = cmd.MarkFlagRequired("url")
	return cmd
}
