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
		Use:         "list",
		Short:       "List the properties this account can access",
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
		Use:         "get",
		Short:       "Get one property's access level",
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
