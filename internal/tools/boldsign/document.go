package boldsign

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/spf13/cobra"
)

// signerTypes is the closed set BoldSign accepts for a signer's role in the
// signing flow.
var signerTypes = map[string]struct{}{
	"Signer":         {},
	"Reviewer":       {},
	"InPersonSigner": {},
}

// actionReceipt is emitted for the 204 endpoints (remind, revoke) that return
// no body — a small confirmation an agent can key on.
type actionReceipt struct {
	OK         bool   `json:"ok"`
	DocumentID string `json:"documentId"`
	Action     string `json:"action"`
}

// downloadReceipt is emitted after a binary download is written to --out.
type downloadReceipt struct {
	OK    bool   `json:"ok"`
	Path  string `json:"path"`
	Bytes int    `json:"bytes"`
}

func (s *Service) newDocumentCmd(token string) *cobra.Command {
	cmd := newGroupCmd("document", "Manage signature documents (send, track, download, remind, revoke)")
	cmd.AddCommand(
		s.newDocumentSendCmd(token),
		s.newDocumentListCmd(token),
		s.newDocumentGetCmd(token),
		s.newDocumentDownloadCmd(token),
		s.newDocumentAuditLogCmd(token),
		s.newDocumentRemindCmd(token),
		s.newDocumentRevokeCmd(token),
	)
	return cmd
}

func (s *Service) newDocumentSendCmd(token string) *cobra.Command {
	var files, fileURLs, signers []string
	var title, message, signerType, onBehalfOf string
	var expiryDays int
	var signingOrder, autoDetectFields, textTags, disableEmails bool
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send files for signature (POST /v1/document/send)",
		Long: "`--file` reads a local path and base64-encodes it into the request body, so\n" +
			"the whole document travels inline; `--file-url` instead hands BoldSign a URL\n" +
			"that BoldSign itself must be able to fetch, which means it has to be\n" +
			"publicly reachable. Both are repeatable and at least one is required, as is\n" +
			"at least one `--signer`. `--title` is required.\n" +
			"\n" +
			"A file with signers but no signature fields is rejected: pass\n" +
			"`--auto-detect-fields` (BoldSign locates the fields itself) or `--text-tags`\n" +
			"(the file already carries BoldSign text tags), or send from a template.\n" +
			"`--signer-type` is a single value applied to EVERY signer — Signer,\n" +
			"Reviewer or InPersonSigner — not a per-signer setting. `--signing-order`\n" +
			"numbers the signers in the order given and makes them sign one after\n" +
			"another. `--disable-emails` suppresses BoldSign's invitation mail entirely,\n" +
			"so nobody learns about the request unless the link is delivered some other\n" +
			"way.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if len(files) == 0 && len(fileURLs) == 0 {
				return &usageError{msg: "boldsign: at least one --file or --file-url is required"}
			}
			if len(signers) == 0 {
				return &usageError{msg: "boldsign: at least one --signer is required"}
			}
			if _, ok := signerTypes[signerType]; !ok {
				return &usageError{msg: fmt.Sprintf("boldsign: --signer-type %q must be one of Signer, Reviewer, InPersonSigner", signerType)}
			}

			body := map[string]any{"Title": title}
			if message != "" {
				body["Message"] = message
			}

			var fileEntries []fileEntry
			for _, path := range files {
				entry, err := readFileEntry(path)
				if err != nil {
					return err
				}
				fileEntries = append(fileEntries, entry)
			}
			if len(fileEntries) > 0 {
				body["Files"] = fileEntries
			}
			if len(fileURLs) > 0 {
				body["FileUrls"] = fileURLs
			}

			signerList := make([]map[string]any, 0, len(signers))
			for i, spec := range signers {
				party, err := parseParty(spec)
				if err != nil {
					return err
				}
				entry := map[string]any{
					"Name":         party.name,
					"EmailAddress": party.email,
					"SignerType":   signerType,
				}
				if signingOrder {
					entry["SignerOrder"] = i + 1
				}
				signerList = append(signerList, entry)
			}
			body["Signers"] = signerList

			if signingOrder {
				body["EnableSigningOrder"] = true
			}
			if expiryDays > 0 {
				body["ExpiryDays"] = expiryDays
			}
			if autoDetectFields {
				body["AutoDetectFields"] = true
			}
			if textTags {
				body["UseTextTags"] = true
			}
			if disableEmails {
				body["DisableEmails"] = true
			}
			if onBehalfOf != "" {
				body["OnBehalfOf"] = onBehalfOf
			}

			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/v1/document/send", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringArrayVar(&files, "file", nil, "local file to send (repeatable; base64-encoded into the request)")
	cmd.Flags().StringArrayVar(&fileURLs, "file-url", nil, "publicly accessible file URL to send (repeatable)")
	cmd.Flags().StringVar(&title, "title", "", "document title (required)")
	cmd.Flags().StringVar(&message, "message", "", "message shown to all recipients")
	cmd.Flags().StringArrayVar(&signers, "signer", nil, "signer as \"Name <email>\" (repeatable, at least one)")
	cmd.Flags().StringVar(&signerType, "signer-type", "Signer", "signer type: Signer|Reviewer|InPersonSigner")
	cmd.Flags().BoolVar(&signingOrder, "signing-order", false, "enforce sequential signing order (assigns SignerOrder by signer order)")
	cmd.Flags().IntVar(&expiryDays, "expiry-days", 0, "days until the request expires")
	cmd.Flags().BoolVar(&autoDetectFields, "auto-detect-fields", false, "auto-detect form fields in the document")
	cmd.Flags().BoolVar(&textTags, "text-tags", false, "parse BoldSign text tags in the document")
	cmd.Flags().BoolVar(&disableEmails, "disable-emails", false, "do not send BoldSign signature emails")
	cmd.Flags().StringVar(&onBehalfOf, "on-behalf-of", "", "sender email to send on behalf of")
	_ = cmd.MarkFlagRequired("title")
	return cmd
}

func (s *Service) newDocumentListCmd(token string) *cobra.Command {
	var statuses []string
	var search, transmitType string
	var page, pageSize int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List / monitor documents (GET /v1/document/list)",
		Long: "`--page` is 1-based and always sent, defaulting to page 1; `--page-size`\n" +
			"falls back to BoldSign's own default of 10. `--status` is repeatable\n" +
			"(Completed, WaitingForOthers, Revoked, ...), `--search` matches title, id or\n" +
			"recipient, and `--transmit-type` is Sent, Received or Both. None of the\n" +
			"three is validated locally, so a misspelled value reaches BoldSign as-is.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("page", strconv.Itoa(page))
			if pageSize > 0 {
				q.Set("pageSize", strconv.Itoa(pageSize))
			}
			for _, status := range statuses {
				q.Add("status", status)
			}
			if search != "" {
				q.Set("searchKey", search)
			}
			if transmitType != "" {
				q.Set("transmitType", transmitType)
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v1/document/list", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&page, "page", 1, "page number (1-based)")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "documents per page (BoldSign default 10)")
	cmd.Flags().StringArrayVar(&statuses, "status", nil, "status filter (repeatable): e.g. Completed, WaitingForOthers, Revoked")
	cmd.Flags().StringVar(&search, "search", "", "search by title, id, or recipient")
	cmd.Flags().StringVar(&transmitType, "transmit-type", "", "transmission filter: Sent|Received|Both")
	return cmd
}

func (s *Service) newDocumentGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get document properties and per-signer status (GET /v1/document/properties)",
		Long: "`--id` is a required flag, not a positional argument. This is the status\n" +
			"read after an asynchronous `document send`, and the way to see which\n" +
			"signers are still outstanding before calling `document remind`.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("documentId", id)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v1/document/properties", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "document id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newDocumentDownloadCmd(token string) *cobra.Command {
	return s.newBinaryDownloadCmd(token, "download",
		"Download the (signed) document PDF (GET /v1/document/download)",
		longDocumentDownload, "/v1/document/download")
}

func (s *Service) newDocumentAuditLogCmd(token string) *cobra.Command {
	return s.newBinaryDownloadCmd(token, "audit-log",
		"Download the audit-trail PDF (GET /v1/document/downloadAuditLog)",
		longDocumentAuditLog, "/v1/document/downloadAuditLog")
}

// longDocumentDownload and longDocumentAuditLog are the two binary-download
// Longs. They live next to the shared builder because it is the builder that
// fixes the required --id/--out pair and the write-to-disk receipt they both
// describe.
const (
	longDocumentDownload = "Writes PDF bytes to `--out`; both `--id` and `--out` are required, and an\n" +
		"existing file at that path is overwritten without warning. stdout carries\n" +
		"only `{\"ok\":true,\"path\":…,\"bytes\":N}` — never the PDF itself. The\n" +
		"document's state is not checked first, so this happily writes a half-signed\n" +
		"copy; call `document get` when a fully executed one is what is wanted."
	longDocumentAuditLog = "Writes the audit trail — the signing history and its timestamps — to\n" +
		"`--out`, NOT the contract; the signed document is `document download`. Both\n" +
		"`--id` and `--out` are required, an existing file at that path is\n" +
		"overwritten without warning, and stdout carries only the receipt."
)

// newBinaryDownloadCmd builds a document/{download,audit-log} command: both GET
// a PDF by documentId and write raw bytes to --out, emitting a receipt.
func (s *Service) newBinaryDownloadCmd(token, use, short, long, path string) *cobra.Command {
	var id, out string
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Long:        long,
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("documentId", id)
			data, err := s.download(cmd.Context(), token, path, q)
			if err != nil {
				return err
			}
			if err := os.WriteFile(out, data, 0o644); err != nil {
				return &usageError{msg: fmt.Sprintf("boldsign: write %s: %v", out, err)}
			}
			return s.emitValue(downloadReceipt{OK: true, Path: out, Bytes: len(data)})
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "document id")
	cmd.Flags().StringVar(&out, "out", "", "path to write the PDF to")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("out")
	return cmd
}

func (s *Service) newDocumentRemindCmd(token string) *cobra.Command {
	var id, message string
	var emails []string
	cmd := &cobra.Command{
		Use:   "remind",
		Short: "Send a reminder to pending signers (POST /v1/document/remind)",
		Long: "`--email` is repeatable and narrows the reminder to those addresses;\n" +
			"omitting it reminds every recipient BoldSign still considers outstanding.\n" +
			"Each call sends real mail and nothing here rate-limits it, so a loop is a\n" +
			"loop of emails. `--message` replaces the default reminder text. There is no\n" +
			"response body — success prints `{\"ok\":true,\"documentId\":…,\"action\":\n" +
			"\"remind\"}`.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("documentId", id)
			for _, email := range emails {
				q.Add("receiverEmails", email)
			}
			var body any
			if message != "" {
				body = map[string]any{"Message": message}
			}
			if _, err := s.call(cmd.Context(), token, http.MethodPost, "/v1/document/remind", q, body); err != nil {
				return err
			}
			return s.emitValue(actionReceipt{OK: true, DocumentID: id, Action: "remind"})
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "document id")
	cmd.Flags().StringArrayVar(&emails, "email", nil, "restrict the reminder to these signer emails (repeatable)")
	cmd.Flags().StringVar(&message, "message", "", "reminder message")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newDocumentRevokeCmd(token string) *cobra.Command {
	var id, message string
	cmd := &cobra.Command{
		Use:   "revoke",
		Short: "Revoke / cancel a document with a reason (POST /v1/document/revoke)",
		Long: "`--message` is required alongside `--id` and is stored as the cancellation\n" +
			"reason. Revoking ends the request for every recipient at once and cannot be\n" +
			"undone — a corrected version has to go out as a NEW document with a new id.\n" +
			"There is no response body; success prints\n" +
			"`{\"ok\":true,\"documentId\":…,\"action\":\"revoke\"}`.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("documentId", id)
			body := map[string]any{"Message": message}
			if _, err := s.call(cmd.Context(), token, http.MethodPost, "/v1/document/revoke", q, body); err != nil {
				return err
			}
			return s.emitValue(actionReceipt{OK: true, DocumentID: id, Action: "revoke"})
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "document id")
	cmd.Flags().StringVar(&message, "message", "", "reason for revoking (required)")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("message")
	return cmd
}
