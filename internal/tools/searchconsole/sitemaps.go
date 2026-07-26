package searchconsole

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

func (s *Service) newSitemapsCmd(token string) *cobra.Command {
	g := newGroupCmd("sitemaps", "Sitemaps for a property (list, get, submit, delete)")
	g.AddCommand(
		s.newSitemapsListCmd(token),
		s.newSitemapsGetCmd(token),
		s.newSitemapsSubmitCmd(token),
		s.newSitemapsDeleteCmd(token),
	)
	return g
}

// sitemapPath builds the escaped .../sites/{site}/sitemaps[/{feed}] path.
func (s *Service) sitemapPath(site, feed string) string {
	p := s.base() + "/sites/" + escapePathSegment(site) + "/sitemaps"
	if feed != "" {
		p += "/" + escapePathSegment(feed)
	}
	return p
}

func (s *Service) newSitemapsListCmd(token string) *cobra.Command {
	var site string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List sitemaps for a property (errors, warnings, last downloaded)",
		Long: "`--site` is required. Each submitted sitemap comes back with its `path`,\n" +
			"error and warning counts, the time Google last downloaded it and the\n" +
			"indexed-URL counts per content type. This is where a sitemap's real\n" +
			"state appears: `sitemaps submit` only confirms that the submission was\n" +
			"accepted, never that Google could fetch or parse the feed.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if site == "" {
				return &usageError{msg: "--site is required"}
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, s.sitemapPath(site, ""), nil)
			if err != nil {
				return err
			}
			var parsed struct {
				Sitemap []json.RawMessage `json:"sitemap"`
			}
			_ = json.Unmarshal(body, &parsed)
			if jsonOut(cmd) {
				return s.emitJSON(map[string]any{"sitemaps": rawArrayOrEmpty(parsed.Sitemap)})
			}
			for _, raw := range parsed.Sitemap {
				var e struct {
					Path     string `json:"path"`
					Errors   string `json:"errors"`
					Warnings string `json:"warnings"`
				}
				_ = json.Unmarshal(raw, &e)
				fmt.Fprintf(s.stdout(), "%s\terrors=%s\twarnings=%s\n", e.Path, e.Errors, e.Warnings)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&site, "site", "", "property URL-prefix or Domain property")
	return cmd
}

func (s *Service) newSitemapsGetCmd(token string) *cobra.Command {
	var site, sitemap string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get one sitemap's status detail",
		Long: "`--site` and `--sitemap` are both required, and `--sitemap` is the FULL\n" +
			"feed URL exactly as `sitemaps list` prints it, not a path fragment.\n" +
			"Returns that one feed's detail — warnings, errors, last download time\n" +
			"and per-type URL counts — which is how you find out, some time after a\n" +
			"submit, whether Google actually read the file.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireSiteSitemap(site, sitemap); err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, s.sitemapPath(site, sitemap), nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&site, "site", "", "property URL-prefix or Domain property")
	cmd.Flags().StringVar(&sitemap, "sitemap", "", "full sitemap feed URL")
	return cmd
}

func (s *Service) newSitemapsSubmitCmd(token string) *cobra.Command {
	var site, sitemap string
	cmd := &cobra.Command{
		Use:   "submit",
		Short: "Submit (or resubmit) a sitemap for a property",
		Long: "Tells Google to crawl a sitemap on the user's live property.\n" +
			"`--sitemap` is the full feed URL and the file must already be reachable\n" +
			"— Google fetches it asynchronously, so acceptance here says nothing\n" +
			"about whether it parsed. The API answers with an empty body, so this\n" +
			"prints a synthesized `{\"ok\":true,…}` and the exit status is the real\n" +
			"signal; read the outcome later with `sitemaps get`. Resubmitting an\n" +
			"already-known sitemap is allowed and simply re-queues it.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireSiteSitemap(site, sitemap); err != nil {
				return err
			}
			if _, err := s.call(cmd.Context(), token, http.MethodPut, s.sitemapPath(site, sitemap), nil); err != nil {
				return err
			}
			return s.emitMutation(cmd, site, sitemap)
		},
	}
	cmd.Flags().StringVar(&site, "site", "", "property URL-prefix or Domain property")
	cmd.Flags().StringVar(&sitemap, "sitemap", "", "full sitemap feed URL")
	return cmd
}

func (s *Service) newSitemapsDeleteCmd(token string) *cobra.Command {
	var site, sitemap string
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a sitemap from a property",
		Long: "Removes the sitemap from the property's submitted list. It does not\n" +
			"delete the file, and it does not deindex anything the sitemap once\n" +
			"listed — those URLs stay in Google's index. `--site` and `--sitemap`\n" +
			"are both required, the latter the full feed URL. The API answers with\n" +
			"an empty body, so this prints a synthesized `{\"ok\":true,…}` and the\n" +
			"exit status is the real signal. Re-adding it later is a plain\n" +
			"`sitemaps submit`.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireSiteSitemap(site, sitemap); err != nil {
				return err
			}
			if _, err := s.call(cmd.Context(), token, http.MethodDelete, s.sitemapPath(site, sitemap), nil); err != nil {
				return err
			}
			return s.emitMutation(cmd, site, sitemap)
		},
	}
	cmd.Flags().StringVar(&site, "site", "", "property URL-prefix or Domain property")
	cmd.Flags().StringVar(&sitemap, "sitemap", "", "full sitemap feed URL")
	return cmd
}

// requireSiteSitemap validates the two flags shared by the feed-scoped verbs.
func requireSiteSitemap(site, sitemap string) error {
	if site == "" {
		return &usageError{msg: "--site is required"}
	}
	if sitemap == "" {
		return &usageError{msg: "--sitemap is required"}
	}
	return nil
}

// emitMutation reports a sitemap submit/delete result. The API returns an empty
// 2xx body, so synthesize a stable {"ok":true,...} envelope echoing the target.
func (s *Service) emitMutation(cmd *cobra.Command, site, sitemap string) error {
	if jsonOut(cmd) {
		return s.emitJSON(map[string]any{"ok": true, "site": site, "sitemap": sitemap})
	}
	fmt.Fprintf(s.stdout(), "ok\t%s\t%s\n", site, sitemap)
	return nil
}
