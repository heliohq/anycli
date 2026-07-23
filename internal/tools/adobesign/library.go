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
		Args:  cobra.NoArgs,
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
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, baseURI, http.MethodGet, "/libraryDocuments/"+url.PathEscape(args[0]), nil)
			if err != nil {
				return err
			}
			return s.emitRaw(body)
		},
	}
}
