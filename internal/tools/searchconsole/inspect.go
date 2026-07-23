package searchconsole

import (
	"net/http"

	"github.com/spf13/cobra"
)

// inspectRequest is the URL Inspection index:inspect body. inspectionUrl and
// siteUrl are required; languageCode (BCP-47) is optional.
type inspectRequest struct {
	InspectionURL string `json:"inspectionUrl"`
	SiteURL       string `json:"siteUrl"`
	LanguageCode  string `json:"languageCode,omitempty"`
}

func (s *Service) newInspectCmd(token string) *cobra.Command {
	var site, pageURL, language string
	cmd := &cobra.Command{
		Use:         "inspect",
		Short:       "URL Inspection: index status and coverage for a page (indexed version only)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if site == "" {
				return &usageError{msg: "--site is required"}
			}
			if pageURL == "" {
				return &usageError{msg: "--url is required"}
			}
			req := inspectRequest{InspectionURL: pageURL, SiteURL: site, LanguageCode: language}
			body, err := s.call(cmd.Context(), token, http.MethodPost, s.inspectBase()+"/urlInspection/index:inspect", req)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	f := cmd.Flags()
	f.StringVar(&site, "site", "", "property URL-prefix or Domain property the URL belongs to")
	f.StringVar(&pageURL, "url", "", "the fully-qualified page URL to inspect")
	f.StringVar(&language, "language", "", "optional BCP-47 language code for the result text")
	return cmd
}
