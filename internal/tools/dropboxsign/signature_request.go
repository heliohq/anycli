package dropboxsign

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// newSendCmd sends a document out for signature. Documents come from either
// --file (uploaded, repeatable -> files[N]) or --file-url (remote URL,
// repeatable -> file_urls[N]); Dropbox Sign requires exactly one of the two.
// Signers are "Name:email" pairs (repeatable); flag order sets each signer's
// signing order.
func (s *Service) newSendCmd(token string) *cobra.Command {
	var (
		files    []string
		fileURLs []string
		signers  []string
		ccs      []string
		title    string
		subject  string
		message  string
		testMode bool
	)
	cmd := &cobra.Command{
		Use:         "send",
		Short:       "Send a document for signature (--file or --file-url)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if len(files) == 0 && len(fileURLs) == 0 {
				return &usageError{msg: "provide at least one --file or --file-url"}
			}
			if len(files) > 0 && len(fileURLs) > 0 {
				return &usageError{msg: "use --file or --file-url, not both"}
			}
			if len(signers) == 0 {
				return &usageError{msg: "provide at least one --signer \"Name:email\""}
			}
			parts, err := buildSignerParts(signers, false)
			if err != nil {
				return err
			}
			parts = append(parts, sharedSendParts(title, subject, message, ccs, testMode)...)
			for i, u := range fileURLs {
				parts = append(parts, formPart{name: fmt.Sprintf("file_urls[%d]", i), value: u})
			}
			for i, p := range files {
				parts = append(parts, formPart{name: fmt.Sprintf("files[%d]", i), filePath: p})
			}
			body, err := s.callMultipart(cmd.Context(), token, "/signature_request/send", parts)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringArrayVar(&files, "file", nil, "path to a local document to upload (repeatable)")
	cmd.Flags().StringArrayVar(&fileURLs, "file-url", nil, "publicly reachable document URL (repeatable)")
	cmd.Flags().StringArrayVar(&signers, "signer", nil, "signer as \"Name:email\" (repeatable; order = signing order)")
	cmd.Flags().StringArrayVar(&ccs, "cc", nil, "CC email address (repeatable)")
	cmd.Flags().StringVar(&title, "title", "", "request title")
	cmd.Flags().StringVar(&subject, "subject", "", "email subject sent to signers")
	cmd.Flags().StringVar(&message, "message", "", "email message sent to signers")
	cmd.Flags().BoolVar(&testMode, "test-mode", false, "create a non-binding, watermarked test request")
	return cmd
}

// newSendWithTemplateCmd sends a request built from one or more saved
// templates. Signers are "Role:Name:email" triples (the role must match a
// template signer role).
func (s *Service) newSendWithTemplateCmd(token string) *cobra.Command {
	var (
		templates []string
		signers   []string
		ccs       []string
		title     string
		subject   string
		message   string
		testMode  bool
	)
	cmd := &cobra.Command{
		Use:         "send-with-template",
		Short:       "Send a signature request from saved template(s)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if len(templates) == 0 {
				return &usageError{msg: "provide at least one --template <template_id>"}
			}
			if len(signers) == 0 {
				return &usageError{msg: "provide at least one --signer \"Role:Name:email\""}
			}
			parts, err := buildSignerParts(signers, true)
			if err != nil {
				return err
			}
			parts = append(parts, sharedSendParts(title, subject, message, ccs, testMode)...)
			for i, id := range templates {
				parts = append(parts, formPart{name: fmt.Sprintf("template_ids[%d]", i), value: id})
			}
			body, err := s.callMultipart(cmd.Context(), token, "/signature_request/send_with_template", parts)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringArrayVar(&templates, "template", nil, "template id (repeatable)")
	cmd.Flags().StringArrayVar(&signers, "signer", nil, "signer as \"Role:Name:email\" (repeatable)")
	cmd.Flags().StringArrayVar(&ccs, "cc", nil, "CC email address (repeatable)")
	cmd.Flags().StringVar(&title, "title", "", "request title")
	cmd.Flags().StringVar(&subject, "subject", "", "email subject sent to signers")
	cmd.Flags().StringVar(&message, "message", "", "email message sent to signers")
	cmd.Flags().BoolVar(&testMode, "test-mode", false, "create a non-binding, watermarked test request")
	return cmd
}

// sharedSendParts builds the send fields common to send and
// send-with-template. test_mode is sent as the boolean literal "true"/"false"
// (not the legacy integer 1/0); cc_email_addresses and the optional
// title/subject/message are omitted when empty.
func sharedSendParts(title, subject, message string, ccs []string, testMode bool) []formPart {
	parts := []formPart{{name: "test_mode", value: strconv.FormatBool(testMode)}}
	if title != "" {
		parts = append(parts, formPart{name: "title", value: title})
	}
	if subject != "" {
		parts = append(parts, formPart{name: "subject", value: subject})
	}
	if message != "" {
		parts = append(parts, formPart{name: "message", value: message})
	}
	for i, cc := range ccs {
		parts = append(parts, formPart{name: fmt.Sprintf("cc_email_addresses[%d]", i), value: cc})
	}
	return parts
}

// buildSignerParts turns "Name:email" (send) or "Role:Name:email"
// (send-with-template) specs into bracket-indexed signer form fields. For the
// plain send, the signing order is the flag index; for templates, the role
// binds the signer to a template slot and no order field is sent.
func buildSignerParts(signers []string, withRole bool) ([]formPart, error) {
	var parts []formPart
	for i, spec := range signers {
		if withRole {
			role, name, email, err := splitRoleSigner(spec)
			if err != nil {
				return nil, err
			}
			parts = append(parts,
				formPart{name: fmt.Sprintf("signers[%d][role]", i), value: role},
				formPart{name: fmt.Sprintf("signers[%d][name]", i), value: name},
				formPart{name: fmt.Sprintf("signers[%d][email_address]", i), value: email},
			)
			continue
		}
		name, email, err := splitNameSigner(spec)
		if err != nil {
			return nil, err
		}
		parts = append(parts,
			formPart{name: fmt.Sprintf("signers[%d][name]", i), value: name},
			formPart{name: fmt.Sprintf("signers[%d][email_address]", i), value: email},
			formPart{name: fmt.Sprintf("signers[%d][order]", i), value: strconv.Itoa(i)},
		)
	}
	return parts, nil
}

// splitNameSigner parses "Name:email" — the name is everything before the last
// colon, the email everything after (an email never contains a colon, so the
// last colon is the reliable separator).
func splitNameSigner(spec string) (name, email string, err error) {
	idx := strings.LastIndex(spec, ":")
	if idx < 0 {
		return "", "", &usageError{msg: fmt.Sprintf("--signer %q must be \"Name:email\"", spec)}
	}
	name = strings.TrimSpace(spec[:idx])
	email = strings.TrimSpace(spec[idx+1:])
	if name == "" || email == "" {
		return "", "", &usageError{msg: fmt.Sprintf("--signer %q must be \"Name:email\"", spec)}
	}
	return name, email, nil
}

// splitRoleSigner parses "Role:Name:email" — the email is after the last colon,
// the role before the first colon, and the name is whatever sits between.
func splitRoleSigner(spec string) (role, name, email string, err error) {
	first := strings.Index(spec, ":")
	last := strings.LastIndex(spec, ":")
	if first < 0 || first == last {
		return "", "", "", &usageError{msg: fmt.Sprintf("--signer %q must be \"Role:Name:email\"", spec)}
	}
	role = strings.TrimSpace(spec[:first])
	name = strings.TrimSpace(spec[first+1 : last])
	email = strings.TrimSpace(spec[last+1:])
	if role == "" || name == "" || email == "" {
		return "", "", "", &usageError{msg: fmt.Sprintf("--signer %q must be \"Role:Name:email\"", spec)}
	}
	return role, name, email, nil
}

// newListCmd lists signature requests visible to this connection, with
// pagination surfaced through --page / --page-size so an agent can walk pages.
func (s *Service) newListCmd(token string) *cobra.Command {
	var (
		page     int
		pageSize int
		query    string
	)
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List signature requests",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if page > 0 {
				q.Set("page", strconv.Itoa(page))
			}
			if pageSize > 0 {
				q.Set("page_size", strconv.Itoa(pageSize))
			}
			if query != "" {
				q.Set("query", query)
			}
			body, err := s.callGET(cmd.Context(), token, "/signature_request/list", q)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().IntVar(&page, "page", 0, "page number (1-based)")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "results per page")
	cmd.Flags().StringVar(&query, "query", "", "filter query (Dropbox Sign search syntax)")
	return cmd
}

// newGetCmd fetches one signature request's status and per-signer state.
func (s *Service) newGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <signature_request_id>",
		Short:       "Get one signature request's status",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.callGET(cmd.Context(), token, "/signature_request/"+url.PathEscape(args[0]), nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// newFilesCmd downloads the request's document(s). Bytes stream to --out (or
// stdout); a small JSON receipt goes to stdout when --out is set so an agent
// gets a machine-readable confirmation instead of raw binary.
func (s *Service) newFilesCmd(token string) *cobra.Command {
	var (
		fileType string
		out      string
	)
	cmd := &cobra.Command{
		Use:         "files <signature_request_id>",
		Short:       "Download the signed document(s) as PDF or ZIP",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			if fileType != "" {
				q.Set("file_type", fileType)
			}
			return s.downloadFiles(cmd.Context(), token, args[0], q, out)
		},
	}
	cmd.Flags().StringVar(&fileType, "file-type", "", "pdf|zip (default: provider default)")
	cmd.Flags().StringVar(&out, "out", "", "write bytes to this path (default: stdout)")
	return cmd
}

// newRemindCmd resends the signing email to one signer.
func (s *Service) newRemindCmd(token string) *cobra.Command {
	var (
		email string
		name  string
	)
	cmd := &cobra.Command{
		Use:         "remind <signature_request_id>",
		Short:       "Remind a signer (resend the signing email)",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			if email == "" {
				return &usageError{msg: "--email is required"}
			}
			payload := map[string]string{"email_address": email}
			if name != "" {
				payload["name"] = name
			}
			body, err := s.callJSON(cmd.Context(), token, "/signature_request/remind/"+url.PathEscape(args[0]), payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "signer email address to remind (required)")
	cmd.Flags().StringVar(&name, "name", "", "signer name (needed only when signers share an email)")
	return cmd
}

// newCancelCmd cancels an incomplete signature request.
func (s *Service) newCancelCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "cancel <signature_request_id>",
		Short:       "Cancel an incomplete signature request",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.callPost(cmd.Context(), token, "/signature_request/cancel/"+url.PathEscape(args[0]))
			if err != nil {
				return err
			}
			// cancel returns 200 with an empty body on success; emit a receipt.
			if strings.TrimSpace(string(body)) == "" {
				return s.emitValue(map[string]any{"cancelled": true, "signature_request_id": args[0]})
			}
			return s.emit(body)
		},
	}
}

// downloadFiles GETs the request's files and either streams them to a path
// (printing a JSON receipt to stdout) or writes them straight to stdout.
func (s *Service) downloadFiles(ctx context.Context, token, id string, q url.Values, out string) error {
	body, err := s.callGET(ctx, token, "/signature_request/files/"+url.PathEscape(id), q)
	if err != nil {
		return err
	}
	if out == "" {
		_, werr := s.stdout().Write(body)
		return werr
	}
	if werr := writeFile(out, body); werr != nil {
		return &apiError{msg: fmt.Sprintf("dropbox-sign: write --out %q: %v", out, werr), err: werr}
	}
	return s.emitValue(map[string]any{
		"signature_request_id": id,
		"path":                 out,
		"bytes":                len(body),
	})
}

// writeFile writes b to path with owner-only permissions (signed documents may
// be confidential).
func writeFile(path string, b []byte) error {
	return os.WriteFile(path, b, 0o600)
}
