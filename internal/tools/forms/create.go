package forms

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newCreateCmd builds `forms create`. The form is always created unpublished:
// the unpublished query param is sent explicitly so the behavior is
// deterministic across Google's 2026-06-30 default flip, and so publishing
// stays a distinct, explicit action the soft guardrail can hang on.
//
// The API's create copies only info.title / info.documentTitle — questions must
// be added in a second step via `batch-update`. The tool does not pretend to
// build a whole form in one call.
func (s *Service) newCreateCmd(token string) *cobra.Command {
	var title, documentTitle string
	cmd := &cobra.Command{
		Use:   "create --title T [--document-title D]",
		Short: "Create an UNPUBLISHED form (title only; add questions with batch-update)",
		Args:  cobra.NoArgs,
		// POST /forms — mutating provider call (design 318).
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if title == "" {
				return fmt.Errorf("forms: --title is required")
			}
			info := map[string]any{"title": title}
			if documentTitle != "" {
				info["documentTitle"] = documentTitle
			}
			payload := map[string]any{"info": info}
			q := url.Values{"unpublished": {"true"}}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/forms", q, payload)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var f struct {
				FormID       string `json:"formId"`
				ResponderURI string `json:"responderUri"`
			}
			if err := json.Unmarshal(body, &f); err != nil {
				return fmt.Errorf("forms: decode created form: %w", err)
			}
			fmt.Fprintf(s.stdout(), "created unpublished form %s\nedit: https://docs.google.com/forms/d/%s/edit\n", f.FormID, f.FormID)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "form title (required)")
	cmd.Flags().StringVar(&documentTitle, "document-title", "", "Drive document title (defaults to the form title)")
	return cmd
}
