package docusign

import (
	"encoding/base64"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// defaultListWindow is how far back `envelope list` looks when --from-date is
// omitted. DocuSign's list endpoint requires a from_date (or explicit ids), so
// a sensible default keeps the command usable without a flag.
const defaultListWindow = 30 * 24 * time.Hour

// defaultAnchor is the anchor string a document-based send places a signature
// tab on. "/sn1/" is DocuSign's documented anchor-tab convention.
const defaultAnchor = "/sn1/"

func (s *Service) newEnvelopeSendCmd(c *apiClient) *cobra.Command {
	var (
		templateID  string
		document    string
		signerEmail string
		signerName  string
		subject     string
		role        string
		anchor      string
		draft       bool
	)
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Create and send an envelope for signature (from a template or a document)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if (templateID == "") == (document == "") {
				return &usageError{msg: "exactly one of --template-id or --document is required"}
			}
			if signerEmail == "" || signerName == "" {
				return &usageError{msg: "--signer-email and --signer-name are required"}
			}
			status := "sent"
			if draft {
				status = "created"
			}
			var payload map[string]any
			if templateID != "" {
				payload = templateSendPayload(templateID, signerEmail, signerName, role, subject, status)
			} else {
				built, err := documentSendPayload(document, signerEmail, signerName, subject, anchor, status)
				if err != nil {
					return err
				}
				payload = built
			}
			body, err := c.callJSON(cmd.Context(), http.MethodPost, "/envelopes", nil, payload)
			if err != nil {
				return err
			}
			var raw rawEnvelopeSummary
			if err := decodeInto(body, &raw); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if jsonMode(cmd) {
				return emitJSON(out, map[string]any{
					"envelope_id": raw.EnvelopeID,
					"status":      raw.Status,
					"uri":         raw.URI,
				})
			}
			emitLine(out, raw.EnvelopeID, raw.Status)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&templateID, "template-id", "", "reusable template id to send (mutually exclusive with --document)")
	f.StringVar(&document, "document", "", "path to a PDF/DOCX to send (mutually exclusive with --template-id)")
	f.StringVar(&signerEmail, "signer-email", "", "signer email (required)")
	f.StringVar(&signerName, "signer-name", "", "signer name (required)")
	f.StringVar(&subject, "subject", "", "email subject (defaults to 'Please sign' for documents)")
	f.StringVar(&role, "role", "Signer", "template role name the signer fills (template sends only)")
	f.StringVar(&anchor, "anchor", defaultAnchor, "anchor text where a signature tab is placed (document sends only)")
	f.BoolVar(&draft, "draft", false, "create the envelope as a draft instead of sending it")
	return cmd
}

// templateSendPayload builds a POST /envelopes body that sends a reusable
// template to one signer via templateRoles.
func templateSendPayload(templateID, email, name, role, subject, status string) map[string]any {
	roleName := role
	if roleName == "" {
		roleName = "Signer"
	}
	payload := map[string]any{
		"templateId": templateID,
		"templateRoles": []map[string]any{
			{"email": email, "name": name, "roleName": roleName},
		},
		"status": status,
	}
	if subject != "" {
		payload["emailSubject"] = subject
	}
	return payload
}

// documentSendPayload builds a POST /envelopes body that sends a local document
// to one signer, placing a signHere tab at the anchor string.
func documentSendPayload(path, email, name, subject, anchor, status string) (map[string]any, error) {
	data, err := os.ReadFile(path) //nolint:gosec // operator-supplied path is intentional
	if err != nil {
		return nil, &usageError{msg: "read --document: " + err.Error()}
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if ext == "" {
		ext = "pdf"
	}
	if subject == "" {
		subject = "Please sign"
	}
	anchorStr := anchor
	if anchorStr == "" {
		anchorStr = defaultAnchor
	}
	return map[string]any{
		"emailSubject": subject,
		"documents": []map[string]any{
			{
				"documentBase64": base64.StdEncoding.EncodeToString(data),
				"name":           filepath.Base(path),
				"fileExtension":  ext,
				"documentId":     "1",
			},
		},
		"recipients": map[string]any{
			"signers": []map[string]any{
				{
					"email":        email,
					"name":         name,
					"recipientId":  "1",
					"routingOrder": "1",
					"tabs": map[string]any{
						"signHereTabs": []map[string]any{
							{"anchorString": anchorStr, "anchorUnits": "pixels", "anchorXOffset": "0", "anchorYOffset": "0"},
						},
					},
				},
			},
		},
		"status": status,
	}, nil
}

func (s *Service) newEnvelopeListCmd(c *apiClient) *cobra.Command {
	var (
		fromDate string
		status   string
		count    int
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recent envelopes by date and status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if fromDate == "" {
				fromDate = time.Now().Add(-defaultListWindow).UTC().Format("2006-01-02")
			}
			query := map[string]string{"from_date": fromDate}
			if status != "" {
				query["status"] = status
			}
			if count > 0 {
				query["count"] = strconv.Itoa(count)
			}
			body, err := c.callJSON(cmd.Context(), http.MethodGet, "/envelopes", query, nil)
			if err != nil {
				return err
			}
			var raw rawEnvelopeList
			if err := decodeInto(body, &raw); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			views := make([]envelopeView, 0, len(raw.Envelopes))
			for _, e := range raw.Envelopes {
				views = append(views, e.view())
			}
			if jsonMode(cmd) {
				return emitJSON(out, map[string]any{"envelopes": views, "count": len(views)})
			}
			for _, v := range views {
				emitLine(out, v.ID, v.Status, v.Subject, v.SentAt)
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&fromDate, "from-date", "", "list envelopes changed on/after this date (YYYY-MM-DD; default 30 days ago)")
	f.StringVar(&status, "status", "", "filter by status (e.g. sent, completed, voided, delivered)")
	f.IntVar(&count, "count", 0, "max number of envelopes to return")
	return cmd
}

func (s *Service) newEnvelopeGetCmd(c *apiClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <envelope-id>",
		Short: "Get one envelope's status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := c.callJSON(cmd.Context(), http.MethodGet, "/envelopes/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			var raw rawEnvelope
			if err := decodeInto(body, &raw); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			v := raw.view()
			if jsonMode(cmd) {
				return emitJSON(out, v)
			}
			emitLine(out, v.ID, v.Status, v.Subject, v.CompletedAt)
			return nil
		},
	}
	return cmd
}

func (s *Service) newEnvelopeRecipientsCmd(c *apiClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recipients <envelope-id>",
		Short: "List an envelope's recipients and per-recipient signing status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := c.callJSON(cmd.Context(), http.MethodGet, "/envelopes/"+url.PathEscape(args[0])+"/recipients", nil, nil)
			if err != nil {
				return err
			}
			var raw rawRecipients
			if err := decodeInto(body, &raw); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			views := raw.views()
			if jsonMode(cmd) {
				return emitJSON(out, map[string]any{"recipients": views})
			}
			for _, v := range views {
				emitLine(out, v.Name, v.Email, v.Status, v.SignedAt)
			}
			return nil
		},
	}
	return cmd
}

func (s *Service) newEnvelopeVoidCmd(c *apiClient) *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "void <envelope-id>",
		Short: "Void an envelope that was sent in error",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(reason) == "" {
				return &usageError{msg: "--reason is required to void an envelope"}
			}
			payload := map[string]any{"status": "voided", "voidedReason": reason}
			body, err := c.callJSON(cmd.Context(), http.MethodPut, "/envelopes/"+url.PathEscape(args[0]), nil, payload)
			if err != nil {
				return err
			}
			var raw rawEnvelopeSummary
			_ = decodeInto(body, &raw)
			out := cmd.OutOrStdout()
			id := raw.EnvelopeID
			if id == "" {
				id = args[0]
			}
			if jsonMode(cmd) {
				return emitJSON(out, map[string]any{"envelope_id": id, "status": "voided"})
			}
			emitLine(out, id, "voided")
			return nil
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "reason the envelope is voided (required)")
	return cmd
}

func (s *Service) newEnvelopeDownloadCmd(c *apiClient) *cobra.Command {
	var outPath string
	cmd := &cobra.Command{
		Use:   "download <envelope-id>",
		Short: "Download the combined completed PDF for an envelope",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := c.callRaw(cmd.Context(), "/envelopes/"+url.PathEscape(args[0])+"/documents/combined")
			if err != nil {
				return err
			}
			if outPath == "" {
				_, werr := cmd.OutOrStdout().Write(body)
				return werr
			}
			if err := os.WriteFile(outPath, body, 0o600); err != nil {
				return &apiError{msg: "docusign: write --out: " + err.Error(), err: err}
			}
			if jsonMode(cmd) {
				return emitJSON(cmd.OutOrStdout(), map[string]any{"envelope_id": args[0], "path": outPath, "bytes": len(body)})
			}
			emitLine(cmd.OutOrStdout(), outPath, strconv.Itoa(len(body))+" bytes")
			return nil
		},
	}
	cmd.Flags().StringVar(&outPath, "out", "", "write the PDF to this path instead of stdout")
	return cmd
}
