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
		Long: "Exactly one of `--template-id` or `--document` is required, and both paths\n" +
			"build a SINGLE-signer envelope: `--signer-email` and `--signer-name` are\n" +
			"required, and there is no way to add a second signer, a Cc recipient or a\n" +
			"routing order from here.\n" +
			"\n" +
			"A template send fills one role, named by `--role`, which defaults to\n" +
			"`Signer` and has to match the template's own role name exactly — a\n" +
			"mismatch leaves the role unfilled rather than erroring loudly. A document\n" +
			"send uploads the local file and places one signature tab wherever the\n" +
			"literal text of `--anchor` (default `/sn1/`) appears in it; a document\n" +
			"that does not contain that string gets NO signature tab, and the signer\n" +
			"receives a document with nothing to sign. `--subject` falls back to\n" +
			"\"Please sign\" for document sends and to the template's own subject\n" +
			"otherwise.\n" +
			"\n" +
			"Without `--draft` the envelope is sent and DocuSign emails the signer\n" +
			"immediately; `--draft` creates it in `created` state instead, which mails\n" +
			"no one but also cannot be sent from this tool afterwards. The response\n" +
			"carries the `envelope_id` every tracking command needs.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
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
		Long: "DocuSign requires a lower date bound, so `--from-date` (YYYY-MM-DD)\n" +
			"defaults to 30 days ago and older envelopes are simply absent until it is\n" +
			"widened. The bound is on when an envelope last CHANGED, not when it was\n" +
			"sent, so a stale envelope that just got signed appears in a narrow window.\n" +
			"`--status` takes one DocuSign status (`sent`, `delivered`, `completed`,\n" +
			"`declined`, `voided`) and `--count` caps the rows. Only summaries come\n" +
			"back — recipients are a separate call.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "Reports the envelope-level state — `status`, `subject`, and the created,\n" +
			"sent and completed timestamps — which says whether the whole thing is\n" +
			"done but not who is holding it up. For that, and for per-signer\n" +
			"timestamps, use `envelope recipients`.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
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
		Long: "This is the \"who has not signed yet\" read: each recipient carries its own\n" +
			"`status` and `signed_at`, plus the `routing_order` that decides who is\n" +
			"even able to act — a recipient later in the order has not been emailed\n" +
			"yet, so an empty status there is expected rather than a stall. Reminders\n" +
			"cannot be sent from this tool.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
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
		Long: "`--reason` is required and DocuSign shows it to the recipients, who are\n" +
			"emailed that the envelope was voided — this is visible to the signer, not\n" +
			"a quiet cleanup. Only an envelope still in flight can be voided; a\n" +
			"`completed` one cannot, and DocuSign rejects the call. Voiding is final:\n" +
			"there is no un-void, and starting over means a fresh `envelope send`.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
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
		Long: "Returns every document in the envelope merged into one PDF, in whatever\n" +
			"state it is currently in — an envelope that is not `completed` still\n" +
			"downloads, just without the signatures that have not happened. `--out`\n" +
			"writes the file to that path; WITHOUT it the raw PDF bytes go to stdout,\n" +
			"which is binary and should not be captured as text. Documents cannot be\n" +
			"downloaded individually here.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
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
