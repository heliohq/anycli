package signnow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/cobra"
)

// rawDoc is the subset of SignNow's document object the tool projects. created
// / updated are unix-second timestamps SignNow renders as JSON strings or
// numbers, so they are captured raw and normalized.
type rawDoc struct {
	ID           string          `json:"id"`
	DocumentName string          `json:"document_name"`
	Created      json.RawMessage `json:"created"`
	Updated      json.RawMessage `json:"updated"`
	Roles        []struct {
		Name string `json:"name"`
	} `json:"roles"`
	FieldInvites []struct {
		ID     string `json:"id"`
		Email  string `json:"email"`
		Role   string `json:"role"`
		Status string `json:"status"`
	} `json:"field_invites"`
	Signatures []json.RawMessage `json:"signatures"`
}

// inviteView is the projected per-signer invite state.
type inviteView struct {
	ID     string `json:"id,omitempty"`
	Email  string `json:"email,omitempty"`
	Role   string `json:"role,omitempty"`
	Status string `json:"status,omitempty"`
}

// docView is the agent-facing projection of a SignNow document — the ids,
// names, roles, invite statuses, and timestamps a teammate acts on, without the
// hundreds of raw field/element lines.
type docView struct {
	ID              string       `json:"id"`
	DocumentName    string       `json:"document_name,omitempty"`
	Created         string       `json:"created,omitempty"`
	Updated         string       `json:"updated,omitempty"`
	Roles           []string     `json:"roles,omitempty"`
	FieldInvites    []inviteView `json:"field_invites,omitempty"`
	SignaturesCount int          `json:"signatures_count"`
}

func projectDoc(d rawDoc) docView {
	v := docView{
		ID:              d.ID,
		DocumentName:    d.DocumentName,
		Created:         rawTimestamp(d.Created),
		Updated:         rawTimestamp(d.Updated),
		SignaturesCount: len(d.Signatures),
	}
	for _, r := range d.Roles {
		if r.Name != "" {
			v.Roles = append(v.Roles, r.Name)
		}
	}
	for _, fi := range d.FieldInvites {
		v.FieldInvites = append(v.FieldInvites, inviteView{
			ID:     fi.ID,
			Email:  fi.Email,
			Role:   fi.Role,
			Status: fi.Status,
		})
	}
	return v
}

// rawTimestamp renders a raw JSON scalar (string or number) as a plain string,
// dropping surrounding quotes.
func rawTimestamp(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return ""
	}
	return strings.Trim(s, `"`)
}

func (s *Service) newDocumentListCmd(token string) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List documents (both modified/in-flight and freshly-uploaded)",
		Long: "SignNow splits documents across two listing endpoints — one for documents\n" +
			"that have been modified or sent, one for those uploaded but not yet\n" +
			"touched — and a freshly-uploaded document lives ONLY in the second until\n" +
			"its first edit. Both are fetched concurrently here and merged deduped by\n" +
			"id, so a document that seems to have vanished right after upload is in\n" +
			"this list; there is no need to guess an id.\n" +
			"\n" +
			"There is no server-side filter or cursor. --limit truncates the merged\n" +
			"result after both endpoints have already been read, so it bounds the\n" +
			"output rather than the work.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			docs, err := s.listDocuments(cmd.Context(), token)
			if err != nil {
				return err
			}
			if limit > 0 && len(docs) > limit {
				docs = docs[:limit]
			}
			return s.emitJSON(map[string]any{"documents": docs})
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "max documents to return (0 = all)")
	return cmd
}

// listDocuments fetches both listing endpoints concurrently and merges them.
// SignNow splits documents across GET /user/documentsv2 (modified: fields,
// texts, or signatures added) and GET /user/documents (not yet modified); a
// just-uploaded doc lives only in the latter until its first edit, so a
// single-endpoint list would hide exactly the docs the "find the doc to act on"
// flow most needs. The merged result is deduped by id (a doc mid-transition can
// appear in both), documentsv2 first.
func (s *Service) listDocuments(ctx context.Context, token string) ([]docView, error) {
	type leg struct {
		docs []rawDoc
		err  error
	}
	var modified, fresh leg
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); modified.docs, modified.err = s.fetchDocs(ctx, token, "/user/documentsv2") }()
	go func() { defer wg.Done(); fresh.docs, fresh.err = s.fetchDocs(ctx, token, "/user/documents") }()
	wg.Wait()
	if modified.err != nil {
		return nil, modified.err
	}
	if fresh.err != nil {
		return nil, fresh.err
	}

	seen := make(map[string]bool)
	out := make([]docView, 0, len(modified.docs)+len(fresh.docs))
	for _, d := range append(modified.docs, fresh.docs...) {
		if d.ID == "" || seen[d.ID] {
			continue
		}
		seen[d.ID] = true
		out = append(out, projectDoc(d))
	}
	return out, nil
}

func (s *Service) fetchDocs(ctx context.Context, token, path string) ([]rawDoc, error) {
	body, err := s.call(ctx, token, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, err
	}
	var docs []rawDoc
	if err := json.Unmarshal(body, &docs); err != nil {
		return nil, &apiError{msg: fmt.Sprintf("signnow: decode %s: %v", path, err), err: err}
	}
	return docs, nil
}

func (s *Service) newDocumentGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <document-id>",
		Short: "Show a document: roles, field invites, statuses, signatures",
		Long: "A projection, not the raw document: roles, one entry per field invite with\n" +
			"its id, email, role and status, plus a count of signatures collected. The\n" +
			"invite ids in `field_invites` are the ONLY place `invite resend` can get\n" +
			"its argument from. Field geometry and element definitions are deliberately\n" +
			"not returned. Read this to answer \"who has signed and who is still\n" +
			"pending\" before nudging anyone.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/document/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			var d rawDoc
			if err := json.Unmarshal(body, &d); err != nil {
				return &apiError{msg: fmt.Sprintf("signnow: decode document: %v", err), err: err}
			}
			return s.emitJSON(projectDoc(d))
		},
	}
	return cmd
}

func (s *Service) newDocumentUploadCmd(token string) *cobra.Command {
	var file, name string
	var extractFields bool
	cmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload a PDF/DOCX to start a signature flow",
		Long: "--file is read whole into memory and posted; --name defaults to the file's\n" +
			"basename and becomes the document name signers see. --extract-fields\n" +
			"routes to a different endpoint that reads text tags already embedded in\n" +
			"the file and turns them into fillable fields, which skips `document\n" +
			"add-fields` entirely for a prepared PDF — without it the document arrives\n" +
			"with no fields at all. Prints the new document id, which everything\n" +
			"downstream needs.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(file) == "" {
				return &usageError{msg: "document upload requires --file"}
			}
			data, err := os.ReadFile(file)
			if err != nil {
				return &usageError{msg: fmt.Sprintf("document upload: read %s: %v", file, err)}
			}
			filename := strings.TrimSpace(name)
			if filename == "" {
				filename = filepath.Base(file)
			}
			path := "/document"
			if extractFields {
				path = "/document/fieldextract"
			}
			body, err := s.callMultipart(cmd.Context(), token, path, nil, "file", filename, data)
			if err != nil {
				return err
			}
			return s.emitID(body)
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "path to the PDF/DOCX to upload (required)")
	cmd.Flags().StringVar(&name, "name", "", "document name (default: basename of --file)")
	cmd.Flags().BoolVar(&extractFields, "extract-fields", false, "extract fillable fields from text tags in the document")
	return cmd
}

func (s *Service) newDocumentAddFieldsCmd(token string) *cobra.Command {
	var fields string
	cmd := &cobra.Command{
		Use:   "add-fields <document-id>",
		Short: "Add fillable fields (signature/text/date) to a document",
		Long: "--fields is a JSON array of field objects carrying x, y, page_number, type\n" +
			"and role. Coordinates are in the document's own space and page_number is\n" +
			"zero-based, so they have to come from the actual layout rather than being\n" +
			"guessed — `document upload --extract-fields` is the alternative when the\n" +
			"file already carries text tags.\n" +
			"\n" +
			"Adding fields changes how the document can be sent: once it has fields,\n" +
			"the free-form `invite send --email` is refused and a role-based `invite\n" +
			"send --to` naming these roles is required.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(fields) == "" {
				return &usageError{msg: "document add-fields requires --fields (a JSON array)"}
			}
			var parsed []any
			if err := json.Unmarshal([]byte(fields), &parsed); err != nil {
				return &usageError{msg: fmt.Sprintf("document add-fields: --fields is not a JSON array: %v", err)}
			}
			body, err := s.call(cmd.Context(), token, http.MethodPut, "/document/"+url.PathEscape(args[0]), nil, map[string]any{"fields": parsed})
			if err != nil {
				return err
			}
			return s.emitID(body)
		},
	}
	cmd.Flags().StringVar(&fields, "fields", "", "JSON array of field objects (x/y/page_number/type/role) (required)")
	return cmd
}

func (s *Service) newDocumentDownloadCmd(token string) *cobra.Command {
	var out string
	var withHistory bool
	cmd := &cobra.Command{
		Use:   "download <document-id>",
		Short: "Download the executed PDF (collapsed)",
		Long: "--out is required; the PDF is written to that path and the command prints\n" +
			"`{\"saved_to\":…,\"bytes\":N}` rather than the file itself. The download is\n" +
			"always the collapsed form — fields flattened into the page — and\n" +
			"--with-history appends SignNow's audit trail, which is what an executed\n" +
			"agreement usually needs for evidence.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(out) == "" {
				return &usageError{msg: "document download requires --out"}
			}
			q := url.Values{"type": {"collapsed"}}
			if withHistory {
				q.Set("with_history", "1")
			}
			data, err := s.callRaw(cmd.Context(), token, "/document/"+url.PathEscape(args[0])+"/download", q)
			if err != nil {
				return err
			}
			if err := os.WriteFile(out, data, 0o644); err != nil {
				return &apiError{msg: fmt.Sprintf("document download: write %s: %v", out, err), err: err}
			}
			return s.emitJSON(map[string]any{"saved_to": out, "bytes": len(data)})
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "output file path (required)")
	cmd.Flags().BoolVar(&withHistory, "with-history", false, "append the document history to the PDF")
	return cmd
}

func (s *Service) newDocumentDeleteCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <document-id>",
		Short: "Delete a document",
		Long: "Permanent, and it takes the signature record with it — the executed PDF\n" +
			"and its audit trail are no longer retrievable afterwards. For anything\n" +
			"that has been signed, run `document download --with-history` first.\n" +
			"Withdrawing an in-flight request without destroying the record is `invite\n" +
			"cancel`.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			if _, err := s.call(cmd.Context(), token, http.MethodDelete, "/document/"+url.PathEscape(id), nil, nil); err != nil {
				return err
			}
			return s.emitJSON(map[string]any{"id": id, "status": "deleted"})
		},
	}
	return cmd
}

// emitID echoes the id SignNow returns from a create/mutate call. SignNow's
// mutation responses carry {"id": "..."}; a body without one still succeeds
// (the HTTP status already confirmed it) and echoes an empty id.
func (s *Service) emitID(body []byte) error {
	var obj struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &obj)
	return s.emitJSON(map[string]any{"id": obj.ID})
}
