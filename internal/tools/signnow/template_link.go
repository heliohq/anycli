package signnow

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

func (s *Service) newTemplateCreateCmd(token string) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "create <document-id>",
		Short: "Turn a document into a reusable template",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(name) == "" {
				return &usageError{msg: "template create requires --name"}
			}
			payload := map[string]any{"document_id": args[0], "document_name": name}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/template", nil, payload)
			if err != nil {
				return err
			}
			return s.emitID(body)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "template name (required)")
	return cmd
}

func (s *Service) newTemplateCopyCmd(token string) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "copy <template-id>",
		Short: "Instantiate a fresh document from a template",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(name) == "" {
				return &usageError{msg: "template copy requires --name"}
			}
			payload := map[string]any{"document_name": name}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/template/"+url.PathEscape(args[0])+"/copy", nil, payload)
			if err != nil {
				return err
			}
			return s.emitID(body)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "name for the new document (required)")
	return cmd
}

func (s *Service) newLinkCreateCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <document-id>",
		Short: "Create a signing link for a document (no known signer email)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{"document_id": args[0]}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/link", nil, payload)
			if err != nil {
				return err
			}
			return s.emitRaw(body)
		},
	}
	return cmd
}
