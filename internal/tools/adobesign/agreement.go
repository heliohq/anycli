package adobesign

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// agreementSummary is the provider-neutral snake_case view of one agreement.
type agreementSummary struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Name    string `json:"name"`
	Created string `json:"created,omitempty"`
}

func (s *Service) newAgreementSendCmd(token, baseURI string) *cobra.Command {
	var document, libraryID, recipientEmail, recipientName, name string
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Create and send an agreement for signature (from a file or a library document)",
		Long: "Exactly one of `--document` (a local file) or `--library-id` (a reusable\n" +
			"library document); both or neither is a usage error. `--document` costs two\n" +
			"calls — the file is uploaded to transient storage first, then the agreement\n" +
			"is created from that id — so sending the same contract repeatedly is cheaper\n" +
			"as a library document. `--recipient-email` and `--name` are required;\n" +
			"`--recipient-name` is optional and cosmetic.\n" +
			"\n" +
			"The agreement is created in state IN_PROCESS, which means it goes out\n" +
			"IMMEDIATELY — there is no draft state to review and no scheduling. Exactly\n" +
			"one signer is supported: this verb builds a single participant set with role\n" +
			"SIGNER and signature type ESIGN, and there is no flag for counter-signers,\n" +
			"CC parties, signing order or form fields. The response carries only the new\n" +
			"`agreement_id`; undoing a send means `agreement cancel`.",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			hasDoc := strings.TrimSpace(document) != ""
			hasLib := strings.TrimSpace(libraryID) != ""
			if hasDoc == hasLib {
				return &usageError{msg: "agreement send requires exactly one of --document or --library-id"}
			}
			if strings.TrimSpace(recipientEmail) == "" {
				return &usageError{msg: "agreement send requires --recipient-email"}
			}
			if strings.TrimSpace(name) == "" {
				return &usageError{msg: "agreement send requires --name"}
			}

			var fileInfo map[string]any
			if hasDoc {
				id, err := s.uploadTransient(cmd.Context(), token, baseURI, document, "", "")
				if err != nil {
					return err
				}
				fileInfo = map[string]any{"transientDocumentId": id}
			} else {
				fileInfo = map[string]any{"libraryDocumentId": libraryID}
			}

			member := map[string]any{"email": recipientEmail}
			if strings.TrimSpace(recipientName) != "" {
				member["name"] = recipientName
			}
			payload := map[string]any{
				"name":          name,
				"fileInfos":     []any{fileInfo},
				"signatureType": "ESIGN",
				"state":         "IN_PROCESS",
				"participantSetsInfo": []any{
					map[string]any{
						"memberInfos": []any{member},
						"order":       1,
						"role":        "SIGNER",
					},
				},
			}
			body, err := s.call(cmd.Context(), token, baseURI, http.MethodPost, "/agreements", payload)
			if err != nil {
				return err
			}
			var out struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(body, &out); err != nil {
				return &apiError{msg: fmt.Sprintf("agreement send: decode response: %v", err), err: err}
			}
			if jsonMode(cmd) {
				return s.emitJSON(map[string]string{"agreement_id": out.ID, "status": "IN_PROCESS"})
			}
			fmt.Fprintln(s.stdout(), out.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&document, "document", "", "local file to send for signature (two-step: uploaded as a transient document)")
	cmd.Flags().StringVar(&libraryID, "library-id", "", "library document id to send instead of a file")
	cmd.Flags().StringVar(&recipientEmail, "recipient-email", "", "signer email address (required)")
	cmd.Flags().StringVar(&recipientName, "recipient-name", "", "signer display name")
	cmd.Flags().StringVar(&name, "name", "", "agreement name (required)")
	return cmd
}

func (s *Service) newAgreementListCmd(token, baseURI string) *cobra.Command {
	var cursor string
	var pageSize int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List agreements (what is out for signature / completed)",
		Long: "Covers agreements the connected user can see, in every state at once — there\n" +
			"is no status, date or counterparty filter, so narrowing is the caller's job\n" +
			"after the fact. Under `--json` each row is trimmed to `id`, `status`, `name`\n" +
			"and `created`, and paging runs off `page_cursor`: feed a non-empty value back\n" +
			"through `--cursor`, sizing pages with `--page-size`.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if strings.TrimSpace(cursor) != "" {
				q.Set("cursor", cursor)
			}
			if pageSize > 0 {
				q.Set("pageSize", fmt.Sprintf("%d", pageSize))
			}
			path := "/agreements"
			if len(q) > 0 {
				path += "?" + q.Encode()
			}
			body, err := s.call(cmd.Context(), token, baseURI, http.MethodGet, path, nil)
			if err != nil {
				return err
			}
			if !jsonMode(cmd) {
				return s.emitRaw(body)
			}
			var resp struct {
				UserAgreementList []struct {
					ID          string `json:"id"`
					Name        string `json:"name"`
					Status      string `json:"status"`
					DisplayDate string `json:"displayDate"`
				} `json:"userAgreementList"`
				Page struct {
					NextCursor string `json:"nextCursor"`
				} `json:"page"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return &apiError{msg: fmt.Sprintf("agreement list: decode response: %v", err), err: err}
			}
			agreements := make([]agreementSummary, 0, len(resp.UserAgreementList))
			for _, a := range resp.UserAgreementList {
				agreements = append(agreements, agreementSummary{ID: a.ID, Status: a.Status, Name: a.Name, Created: a.DisplayDate})
			}
			return s.emitJSON(map[string]any{"agreements": agreements, "page_cursor": resp.Page.NextCursor})
		},
	}
	cmd.Flags().StringVar(&cursor, "cursor", "", "page cursor for the next page")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "max agreements per page")
	return cmd
}

func (s *Service) newAgreementGetCmd(token, baseURI string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <agreement-id>",
		Short: "Get one agreement's status",
		Long: "Under `--json` this returns the same four fields `agreement list` already\n" +
			"gives — `id`, `status`, `name`, `created` — so it is worth calling to poll\n" +
			"one agreement, not to enrich a listed one. Per-signer progress is a\n" +
			"different call: `agreement members`. Without `--json` the full provider\n" +
			"payload comes through instead, which is where anything beyond those four\n" +
			"fields lives.",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, baseURI, http.MethodGet, "/agreements/"+url.PathEscape(args[0]), nil)
			if err != nil {
				return err
			}
			if !jsonMode(cmd) {
				return s.emitRaw(body)
			}
			var a struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				Status      string `json:"status"`
				DisplayDate string `json:"displayDate"`
			}
			if err := json.Unmarshal(body, &a); err != nil {
				return &apiError{msg: fmt.Sprintf("agreement get: decode response: %v", err), err: err}
			}
			return s.emitJSON(agreementSummary{ID: a.ID, Status: a.Status, Name: a.Name, Created: a.DisplayDate})
		},
	}
}

func (s *Service) newAgreementMembersCmd(token, baseURI string) *cobra.Command {
	return &cobra.Command{
		Use:   "members <agreement-id>",
		Short: "List per-participant signing status",
		Long: "Answers who is holding things up, which the agreement-level `status` cannot.\n" +
			"Under `--json` it flattens Adobe's participant sets into one list of\n" +
			"`{email, status, order, role}`; `order` is the signing position, so a\n" +
			"participant later in the sequence stays untouched until the earlier ones\n" +
			"finish. A participant with no status of its own inherits its set's.",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, baseURI, http.MethodGet, "/agreements/"+url.PathEscape(args[0])+"/members", nil)
			if err != nil {
				return err
			}
			if !jsonMode(cmd) {
				return s.emitRaw(body)
			}
			var resp struct {
				ParticipantSets []struct {
					Order       int    `json:"order"`
					Role        string `json:"role"`
					Status      string `json:"status"`
					MemberInfos []struct {
						Email  string `json:"email"`
						Status string `json:"status"`
					} `json:"memberInfos"`
				} `json:"participantSets"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return &apiError{msg: fmt.Sprintf("agreement members: decode response: %v", err), err: err}
			}
			type participant struct {
				Email  string `json:"email"`
				Status string `json:"status"`
				Order  int    `json:"order"`
				Role   string `json:"role"`
			}
			participants := make([]participant, 0, len(resp.ParticipantSets))
			for _, set := range resp.ParticipantSets {
				for _, m := range set.MemberInfos {
					status := m.Status
					if status == "" {
						status = set.Status
					}
					participants = append(participants, participant{Email: m.Email, Status: status, Order: set.Order, Role: set.Role})
				}
			}
			return s.emitJSON(map[string]any{"participants": participants})
		},
	}
}

func (s *Service) newAgreementCancelCmd(token, baseURI string) *cobra.Command {
	var comment string
	cmd := &cobra.Command{
		Use:   "cancel <agreement-id>",
		Short: "Cancel a sent agreement (sender-initiated)",
		Long: "Sender-initiated cancellation, terminal: the agreement moves to CANCELLED\n" +
			"and cannot be reopened or resent — a replacement is a fresh `agreement send`.\n" +
			"It only applies while the agreement is still in flight; one that already\n" +
			"reached SIGNED is rejected by Adobe. `--comment` is recorded on the\n" +
			"agreement for the audit trail. Participants are NOT notified: the call pins\n" +
			"`notifyOthers` to false, so anyone already emailed a signing link has to be\n" +
			"told separately.",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cancelInfo := map[string]any{"notifyOthers": false}
			if strings.TrimSpace(comment) != "" {
				cancelInfo["comment"] = comment
			}
			payload := map[string]any{"state": "CANCELLED", "agreementCancellationInfo": cancelInfo}
			_, err := s.call(cmd.Context(), token, baseURI, http.MethodPut, "/agreements/"+url.PathEscape(args[0])+"/state", payload)
			if err != nil {
				return err
			}
			if jsonMode(cmd) {
				return s.emitJSON(map[string]string{"agreement_id": args[0], "status": "CANCELLED"})
			}
			fmt.Fprintln(s.stdout(), args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&comment, "comment", "", "cancellation reason recorded on the agreement")
	return cmd
}

func (s *Service) newAgreementDownloadCmd(token, baseURI string) *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "download <agreement-id>",
		Short: "Download the combined signed PDF",
		Long: "Fetches the combined document — the signed original plus Adobe's audit\n" +
			"trail — as a single PDF. Without `--out` the raw PDF BYTES go to stdout,\n" +
			"which is rarely what a caller wants; pass `--out <path>` to write a file and\n" +
			"get the path back instead. Only meaningful once `agreement get` reports\n" +
			"SIGNED; earlier states have no combined document to return.",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := s.download(cmd.Context(), token, baseURI, "/agreements/"+url.PathEscape(args[0])+"/combinedDocument", out); err != nil {
				return err
			}
			if strings.TrimSpace(out) != "" && jsonMode(cmd) {
				return s.emitJSON(map[string]string{"agreement_id": args[0], "path": out})
			}
			if strings.TrimSpace(out) != "" {
				fmt.Fprintln(s.stdout(), out)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "write the PDF to this path (default: stdout)")
	return cmd
}

// emitRaw writes a provider response body verbatim to stdout (plain mode).
func (s *Service) emitRaw(body []byte) error {
	out := body
	if len(out) == 0 || out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	_, err := s.stdout().Write(out)
	return err
}
