package pandadoc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/spf13/cobra"
)

// Document status values relevant to the create-then-send loop. A newly created
// document is returned as "document.uploaded" and becomes sendable only once
// background processing flips it to "document.draft".
const (
	statusUploaded = "document.uploaded"
	statusDraft    = "document.draft"
)

// pollInterval / pollMaxAttempts bound the create draft-wait loop: a fixed
// interval (no backoff) and a hard attempt cap (~60s) after which the caller
// gets the document id back plus an explicit timeout error. pollInterval is a
// var so tests can shorten it.
var (
	pollInterval    = 2 * time.Second
	pollMaxAttempts = 30
)

// sessionLinkPrefix builds the shareable signing URL from a session id.
const sessionLinkPrefix = "https://app.pandadoc.com/s/"

func (s *Service) newDocumentListCmd(authz string) *cobra.Command {
	var q, status, template, folder, order string
	var count, page int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List documents (filter by query, status, template, folder)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			query := url.Values{}
			setIf(query, "q", q)
			setIf(query, "status", status)
			setIf(query, "template_id", template)
			setIf(query, "folder_uuid", folder)
			setIf(query, "ordering", order)
			if cmd.Flags().Changed("count") {
				query.Set("count", fmt.Sprintf("%d", count))
			}
			if cmd.Flags().Changed("page") {
				query.Set("page", fmt.Sprintf("%d", page))
			}
			body, err := s.call(cmd.Context(), authz, http.MethodGet, "/documents", query, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitJSON(body)
			}
			return s.renderList(body)
		},
	}
	cmd.Flags().StringVar(&q, "q", "", "search by document name")
	cmd.Flags().StringVar(&status, "status", "", "filter by status, e.g. document.sent")
	cmd.Flags().StringVar(&template, "template", "", "filter by template id")
	cmd.Flags().StringVar(&folder, "folder", "", "filter by folder uuid")
	cmd.Flags().StringVar(&order, "order", "", "ordering field, e.g. date_created")
	cmd.Flags().IntVar(&count, "count", 0, "max results per page")
	cmd.Flags().IntVar(&page, "page", 0, "1-based page number")
	return cmd
}

func (s *Service) newDocumentCreateCmd(authz string) *cobra.Command {
	var template, name, body, bodyFile string
	var recipients, tokens, fields, metadata []string
	var noWait bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a document from a template",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := s.buildCreatePayload(cmd, template, name, body, bodyFile, recipients, tokens, fields, metadata)
			if err != nil {
				return err
			}
			created, err := s.call(cmd.Context(), authz, http.MethodPost, "/documents", nil, payload)
			if err != nil {
				return err
			}
			final := created
			if !noWait {
				final, err = s.waitForDraft(cmd.Context(), authz, created)
				if err != nil {
					return err
				}
			}
			if jsonOut(cmd) {
				return s.emitJSON(final)
			}
			return s.renderItem(final)
		},
	}
	cmd.Flags().StringVar(&template, "template", "", "template uuid to create from")
	cmd.Flags().StringVar(&name, "name", "", "document name")
	cmd.Flags().StringArrayVar(&recipients, "recipient", nil, "recipient as email[:role[:first[:last]]] (repeatable)")
	cmd.Flags().StringArrayVar(&tokens, "token", nil, "template variable as name=value (repeatable)")
	cmd.Flags().StringArrayVar(&fields, "field", nil, "merge field as name=value (repeatable)")
	cmd.Flags().StringArrayVar(&metadata, "metadata", nil, "metadata entry as key=value (repeatable)")
	cmd.Flags().BoolVar(&noWait, "no-wait", false, "return immediately without waiting for the document to become a draft")
	cmd.Flags().StringVar(&body, "body", "", "raw JSON create payload (mutually exclusive with the structured flags)")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "read the raw JSON create payload from a file")
	return cmd
}

// buildCreatePayload assembles the create body from either the raw --body /
// --body-file escape hatch or the structured flags — never both.
func (s *Service) buildCreatePayload(cmd *cobra.Command, template, name, body, bodyFile string, recipients, tokens, fields, metadata []string) (any, error) {
	hasBody := cmd.Flags().Changed("body") || cmd.Flags().Changed("body-file")
	hasFlags := template != "" || name != "" || len(recipients) > 0 || len(tokens) > 0 || len(fields) > 0 || len(metadata) > 0
	if hasBody && hasFlags {
		return nil, &usageError{msg: "document create: --body/--body-file and the structured flags are mutually exclusive"}
	}
	if cmd.Flags().Changed("body") && cmd.Flags().Changed("body-file") {
		return nil, &usageError{msg: "document create: --body and --body-file are mutually exclusive"}
	}
	if hasBody {
		raw := []byte(body)
		if cmd.Flags().Changed("body-file") {
			b, err := os.ReadFile(bodyFile)
			if err != nil {
				return nil, &usageError{msg: fmt.Sprintf("document create: read --body-file %s: %v", bodyFile, err)}
			}
			raw = b
		}
		var v json.RawMessage
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, &usageError{msg: fmt.Sprintf("document create: --body is not valid JSON: %v", err)}
		}
		return v, nil
	}

	if template == "" {
		return nil, &usageError{msg: "document create: --template is required (or pass --body)"}
	}
	if len(recipients) == 0 {
		return nil, &usageError{msg: "document create: at least one --recipient is required (or pass --body)"}
	}
	recips := make([]recipient, 0, len(recipients))
	for _, raw := range recipients {
		r, err := parseRecipient(raw)
		if err != nil {
			return nil, err
		}
		recips = append(recips, r)
	}
	toks, err := buildTokens(tokens)
	if err != nil {
		return nil, err
	}
	fieldObj, err := buildFields(fields)
	if err != nil {
		return nil, err
	}
	metaObj, err := buildMetadata(metadata)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"template_uuid": template,
		"recipients":    recips,
	}
	if name != "" {
		payload["name"] = name
	}
	if len(toks) > 0 {
		payload["tokens"] = toks
	}
	if len(fieldObj) > 0 {
		payload["fields"] = fieldObj
	}
	if len(metaObj) > 0 {
		payload["metadata"] = metaObj
	}
	return payload, nil
}

// waitForDraft polls GET /documents/{id} until the document leaves
// "document.uploaded" (normally reaching "document.draft"), so a following
// `send` does not hit a still-processing document. On the attempt cap it writes
// the id to stdout (for a manual `document status`) and returns a timeout error.
func (s *Service) waitForDraft(ctx context.Context, authz string, created []byte) ([]byte, error) {
	var doc docSummary
	if err := json.Unmarshal(created, &doc); err != nil {
		return created, nil // unexpected shape; return the create response as-is
	}
	id := doc.id()
	if id == "" || doc.Status != statusUploaded {
		return created, nil // already draft/other terminal state, or no id to poll
	}
	for attempt := 0; attempt < pollMaxAttempts; attempt++ {
		body, err := s.call(ctx, authz, http.MethodGet, "/documents/"+url.PathEscape(id), nil, nil)
		if err != nil {
			return nil, err
		}
		var d docSummary
		_ = json.Unmarshal(body, &d)
		if d.Status != statusUploaded {
			return body, nil
		}
		select {
		case <-ctx.Done():
			return nil, &apiError{msg: fmt.Sprintf("pandadoc: wait for draft %s: %v", id, ctx.Err()), err: ctx.Err()}
		case <-time.After(pollInterval):
		}
	}
	fmt.Fprintln(s.stdout(), id)
	return nil, &apiError{msg: fmt.Sprintf(
		"document %s did not reach draft after %d polls; run `document status %s` to keep checking", id, pollMaxAttempts, id)}
}

func (s *Service) newDocumentStatusCmd(authz string) *cobra.Command {
	return &cobra.Command{
		Use:   "status <id>",
		Short: "Show a document's status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), authz, http.MethodGet, "/documents/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitJSON(body)
			}
			return s.renderItem(body)
		},
	}
}

func (s *Service) newDocumentDetailsCmd(authz string) *cobra.Command {
	return &cobra.Command{
		Use:   "details <id>",
		Short: "Show a document's full details (recipients, fields, tokens, dates)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), authz, http.MethodGet, "/documents/"+url.PathEscape(args[0])+"/details", nil, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitJSON(body)
			}
			return s.renderItem(body)
		},
	}
}

func (s *Service) newDocumentSendCmd(authz string) *cobra.Command {
	var subject, message string
	var silent bool
	cmd := &cobra.Command{
		Use:   "send <id>",
		Short: "Send a document for signature",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{}
			if subject != "" {
				payload["subject"] = subject
			}
			if message != "" {
				payload["message"] = message
			}
			if silent {
				payload["silent"] = true
			}
			body, err := s.call(cmd.Context(), authz, http.MethodPost, "/documents/"+url.PathEscape(args[0])+"/send", nil, payload)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitJSON(body)
			}
			return s.renderItem(body)
		},
	}
	cmd.Flags().StringVar(&subject, "subject", "", "email subject")
	cmd.Flags().StringVar(&message, "message", "", "email message body")
	cmd.Flags().BoolVar(&silent, "silent", false, "send without emailing recipients")
	return cmd
}

func (s *Service) newDocumentLinkCmd(authz string) *cobra.Command {
	var email string
	var lifetime int
	cmd := &cobra.Command{
		Use:   "link <id>",
		Short: "Create a shareable signing session link for a recipient",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{"recipient": email}
			if cmd.Flags().Changed("lifetime") {
				payload["lifetime"] = lifetime
			}
			body, err := s.call(cmd.Context(), authz, http.MethodPost, "/documents/"+url.PathEscape(args[0])+"/session", nil, payload)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitJSON(body)
			}
			var sess struct {
				ID        string `json:"id"`
				ExpiresAt string `json:"expires_at"`
			}
			if err := json.Unmarshal(body, &sess); err != nil || sess.ID == "" {
				return s.emitJSON(body)
			}
			fmt.Fprintf(s.stdout(), "%s%s\t%s\n", sessionLinkPrefix, sess.ID, sess.ExpiresAt)
			return nil
		},
	}
	cmd.Flags().StringVar(&email, "recipient", "", "recipient email the session is for")
	cmd.Flags().IntVar(&lifetime, "lifetime", 0, "link lifetime in seconds (default provider-side 3600)")
	_ = cmd.MarkFlagRequired("recipient")
	return cmd
}

func (s *Service) newDocumentDownloadCmd(authz string) *cobra.Command {
	var out string
	var protected bool
	cmd := &cobra.Command{
		Use:   "download <id>",
		Short: "Download a document's PDF to a file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "/documents/" + url.PathEscape(args[0]) + "/download"
			if protected {
				path = "/documents/" + url.PathEscape(args[0]) + "/download-protected"
			}
			data, err := s.download(cmd.Context(), authz, path)
			if err != nil {
				return err
			}
			if err := os.WriteFile(out, data, 0o644); err != nil {
				return &apiError{msg: fmt.Sprintf("pandadoc: write %s: %v", out, err), err: err}
			}
			if jsonOut(cmd) {
				return s.emitValue(map[string]any{"path": out, "bytes": len(data)})
			}
			fmt.Fprintf(s.stdout(), "saved %s (%d bytes)\n", out, len(data))
			return nil
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "output file path")
	cmd.Flags().BoolVar(&protected, "protected", false, "download the certified/signed PDF (download-protected)")
	_ = cmd.MarkFlagRequired("out")
	return cmd
}

func (s *Service) newDocumentDeleteCmd(authz string) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a document",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			_, err := s.call(cmd.Context(), authz, http.MethodDelete, "/documents/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitValue(map[string]any{"id": id, "deleted": true})
			}
			fmt.Fprintf(s.stdout(), "deleted %s\n", id)
			return nil
		},
	}
}

// setIf sets a query key only when the value is non-empty.
func setIf(q url.Values, key, val string) {
	if val != "" {
		q.Set(key, val)
	}
}
