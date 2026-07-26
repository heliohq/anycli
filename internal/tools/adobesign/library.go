package adobesign

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newLibraryListCmd(token, baseURI string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List reusable library documents (templates)",
		Long: "The source of the `--library-id` that `agreement send` takes. Under `--json`\n" +
			"each entry is just an id and a name, with no paging or filter flags — the\n" +
			"whole library comes back at once. Templates are authored in Adobe Acrobat\n" +
			"Sign; nothing here creates or edits one.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, baseURI, http.MethodGet, "/libraryDocuments", nil)
			if err != nil {
				return err
			}
			if !jsonMode(cmd) {
				return s.emitRaw(body)
			}
			var resp struct {
				LibraryDocumentList []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"libraryDocumentList"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return &apiError{msg: fmt.Sprintf("library list: decode response: %v", err), err: err}
			}
			type libraryDoc struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			}
			docs := make([]libraryDoc, 0, len(resp.LibraryDocumentList))
			for _, d := range resp.LibraryDocumentList {
				docs = append(docs, libraryDoc{ID: d.ID, Name: d.Name})
			}
			return s.emitJSON(map[string]any{"library_documents": docs})
		},
	}
}

func (s *Service) newLibraryGetCmd(token, baseURI string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <library-document-id>",
		Short: "Get one library document",
		Long: "The one command that always prints Adobe's raw payload — `--json` does not\n" +
			"reshape it — so expect camelCase provider fields rather than this tool's\n" +
			"snake_case envelope. Use it to confirm a template's identity and settings\n" +
			"before committing it to an `agreement send --library-id`.",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, baseURI, http.MethodGet, "/libraryDocuments/"+url.PathEscape(args[0]), nil)
			if err != nil {
				return err
			}
			return s.emitRaw(body)
		},
	}
}
