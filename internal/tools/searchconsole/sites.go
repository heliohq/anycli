package searchconsole

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

func (s *Service) newSitesCmd(token string) *cobra.Command {
	g := newGroupCmd("sites", "Search Console properties (list, get access level)")
	g.AddCommand(s.newSitesListCmd(token), s.newSitesGetCmd(token))
	return g
}

func (s *Service) newSitesListCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the properties this account can access",
		Long: "The only command needing no `--site`, and the source of the exact\n" +
			"property strings the others require — `https://example.com/` or\n" +
			"`sc-domain:example.com`, to be copied verbatim. Each entry's\n" +
			"`permissionLevel` (`siteOwner`, `siteFullUser`, `siteRestrictedUser`,\n" +
			"`siteUnverifiedUser`) decides what will work: a restricted user can\n" +
			"read performance data but cannot submit or delete a sitemap.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, s.base()+"/sites", nil)
			if err != nil {
				return err
			}
			var parsed struct {
				SiteEntry []json.RawMessage `json:"siteEntry"`
			}
			_ = json.Unmarshal(body, &parsed)
			if jsonOut(cmd) {
				return s.emitJSON(map[string]any{"sites": rawArrayOrEmpty(parsed.SiteEntry)})
			}
			for _, raw := range parsed.SiteEntry {
				var e struct {
					SiteURL         string `json:"siteUrl"`
					PermissionLevel string `json:"permissionLevel"`
				}
				_ = json.Unmarshal(raw, &e)
				fmt.Fprintf(s.stdout(), "%s\t%s\n", e.SiteURL, e.PermissionLevel)
			}
			return nil
		},
	}
}

func (s *Service) newSitesGetCmd(token string) *cobra.Command {
	var site string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get one property's access level",
		Long: "`--site` is required and must be the exact string `sites list` prints.\n" +
			"Returns that property's `permissionLevel`, which is the cheap way to\n" +
			"find out whether a sitemap write will be allowed before attempting one.\n" +
			"A property this account cannot see comes back as a 403 rather than an\n" +
			"empty result.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if site == "" {
				return &usageError{msg: "--site is required"}
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, s.base()+"/sites/"+escapePathSegment(site), nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&site, "site", "", "property URL-prefix (https://example.com/) or Domain property (sc-domain:example.com)")
	return cmd
}
