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
		Long: "Promotes an existing document, with whatever fields and roles it already\n" +
			"carries, into a reusable template; --name is required and the source\n" +
			"document is left alone. A template is not signed itself — each agreement\n" +
			"begins with `template copy`, so getting the fields right once here saves\n" +
			"repeating `document add-fields` per deal.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
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
		Long: "Produces a NEW document with its own id, carrying the template's fields\n" +
			"and roles; --name is required and names that new document. The template is\n" +
			"never mutated by the resulting signature flow, so it stays reusable. The\n" +
			"printed id is a document id — pass it to `invite send`, not to `template\n" +
			"copy` again.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
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
		Long: "Mints a URL that ANYONE holding it can sign with — there is no recipient\n" +
			"address and no identity check, so the link itself is the credential and\n" +
			"should be shared only as the user intends. Use it when the signer's email\n" +
			"is unknown; when it is known, `invite send` binds the signature to a named\n" +
			"person and gives a trackable invite status instead.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
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
