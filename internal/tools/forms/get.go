package forms

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <form-id>",
		Short: "Show a form: structure, publishSettings, responderUri, linkedSheetId (forms.get)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			formID, err := extractFormID(args[0])
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/forms/"+url.PathEscape(formID), nil, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			return s.renderForm(body)
		},
	}
}

// renderForm prints the human-readable summary of a Form resource.
func (s *Service) renderForm(body []byte) error {
	var f struct {
		FormID string `json:"formId"`
		Info   struct {
			Title         string `json:"title"`
			DocumentTitle string `json:"documentTitle"`
			Description   string `json:"description"`
		} `json:"info"`
		ResponderURI    string `json:"responderUri"`
		LinkedSheetID   string `json:"linkedSheetId"`
		PublishSettings *struct {
			PublishState *struct {
				IsPublished          bool `json:"isPublished"`
				IsAcceptingResponses bool `json:"isAcceptingResponses"`
			} `json:"publishState"`
		} `json:"publishSettings"`
		Items []struct {
			ItemID string `json:"itemId"`
			Title  string `json:"title"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &f); err != nil {
		return fmt.Errorf("forms: decode form: %w", err)
	}
	fmt.Fprintf(s.stdout(), "FormId:        %s\nTitle:         %s\n", f.FormID, f.Info.Title)
	if f.Info.DocumentTitle != "" {
		fmt.Fprintf(s.stdout(), "DocumentTitle: %s\n", f.Info.DocumentTitle)
	}
	if f.PublishSettings != nil && f.PublishSettings.PublishState != nil {
		st := f.PublishSettings.PublishState
		fmt.Fprintf(s.stdout(), "Published:     %t\nAccepting:     %t\n", st.IsPublished, st.IsAcceptingResponses)
	} else {
		fmt.Fprintln(s.stdout(), "Published:     (no publishSettings — unpublished or a legacy form)")
	}
	if f.ResponderURI != "" {
		fmt.Fprintf(s.stdout(), "ResponderUri:  %s\n", f.ResponderURI)
	}
	if f.LinkedSheetID != "" {
		fmt.Fprintf(s.stdout(), "LinkedSheet:   %s\n", f.LinkedSheetID)
	}
	fmt.Fprintf(s.stdout(), "Items:         %d\n", len(f.Items))
	for _, it := range f.Items {
		fmt.Fprintf(s.stdout(), "  %s\t%s\n", it.ItemID, it.Title)
	}
	return nil
}
