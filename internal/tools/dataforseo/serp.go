package dataforseo

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newSERPCmd is the `serp` resource group.
func (s *Service) newSERPCmd(credential string) *cobra.Command {
	serp := newGroupCmd("serp", "Search engine results (SERP)")
	serp.AddCommand(s.newSERPGoogleCmd(credential))
	return serp
}

// newSERPGoogleCmd runs a live Google organic SERP for a single keyword.
func (s *Service) newSERPGoogleCmd(credential string) *cobra.Command {
	var (
		keyword string
		tp      taskParams
		depth   int
		device  string
	)
	cmd := &cobra.Command{
		Use:   "google",
		Short: "Live Google organic SERP for a keyword",
		Long: "Fetches the live organic results with their SERP features. --depth is how\n" +
			"many results to return (default 10, max 200) and costs more as it grows,\n" +
			"because each block of results is another page fetched. --device is desktop\n" +
			"(default) or mobile, and the two rank differently often enough to be worth\n" +
			"choosing deliberately. A --keyword carrying a search operator such as\n" +
			"`site:` or `inurl:` costs roughly FIVE TIMES a plain keyword, so add one\n" +
			"only when the question needs it.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			task := map[string]any{"keyword": keyword}
			tp.apply(task)
			if depth > 0 {
				task["depth"] = depth
			}
			if device != "" {
				task["device"] = device
			}
			return s.do(cmd.Context(), credential, http.MethodPost, "/serp/google/organic/live/advanced", task)
		},
	}
	cmd.Annotations = readOnly
	cmd.Flags().StringVar(&keyword, "keyword", "", "search query (required)")
	_ = cmd.MarkFlagRequired("keyword")
	registerLocationLang(cmd, &tp)
	cmd.Flags().IntVar(&depth, "depth", 0, "number of results to return (default 10, max 200)")
	cmd.Flags().StringVar(&device, "device", "", "desktop or mobile (default desktop)")
	return cmd
}
