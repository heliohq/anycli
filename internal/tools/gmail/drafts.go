package gmail

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// draftPayload builds the drafts create/update request body from compose
// options.
func draftPayload(o *composeOptions) (map[string]any, error) {
	body, err := o.resolveComposeBody()
	if err != nil {
		return nil, err
	}
	raw, err := buildMIME(mimeMessage{
		to: o.to, cc: o.cc, bcc: o.bcc,
		subject: o.subject, body: body, html: o.html,
		attachments: o.attachments,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"message": map[string]string{"raw": rawField(raw)}}, nil
}

func (s *Service) newDraftsCreateCmd(token string) *cobra.Command {
	var o composeOptions
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a draft (same parameters as messages send)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := draftPayload(&o)
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/users/me/drafts", nil, payload)
			if err != nil {
				return err
			}
			return s.emitDraft(cmd, body, "created draft")
		},
	}
	addAddressFlags(cmd, &o)
	addBodyFlags(cmd, &o)
	return cmd
}

func (s *Service) newDraftsUpdateCmd(token string) *cobra.Command {
	var o composeOptions
	cmd := &cobra.Command{
		Use:   "update <draft-id>",
		Short: "Replace a draft's content (same parameters as messages send)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := draftPayload(&o)
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodPut, "/users/me/drafts/"+url.PathEscape(args[0]), nil, payload)
			if err != nil {
				return err
			}
			return s.emitDraft(cmd, body, "updated draft")
		},
	}
	addAddressFlags(cmd, &o)
	addBodyFlags(cmd, &o)
	return cmd
}

func (s *Service) newDraftsListCmd(token string) *cobra.Command {
	var pageToken string
	var max int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List drafts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("maxResults", strconv.Itoa(max))
			if pageToken != "" {
				q.Set("pageToken", pageToken)
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/users/me/drafts", q, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				Drafts []struct {
					ID      string `json:"id"`
					Message struct {
						ID       string `json:"id"`
						ThreadID string `json:"threadId"`
					} `json:"message"`
				} `json:"drafts"`
				NextPageToken string `json:"nextPageToken"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("gmail: decode draft list: %w", err)
			}
			if len(resp.Drafts) == 0 {
				fmt.Fprintln(s.stdout(), "no drafts")
				return nil
			}
			for _, d := range resp.Drafts {
				fmt.Fprintf(s.stdout(), "%s\tmessage=%s\n", d.ID, d.Message.ID)
			}
			if resp.NextPageToken != "" {
				fmt.Fprintf(s.stdout(), "next page token: %s\n", resp.NextPageToken)
			}
			return nil
		},
	}
	addListFlags(cmd, &max, &pageToken)
	return cmd
}

func (s *Service) newDraftsGetCmd(token string) *cobra.Command {
	var bodyKind string
	cmd := &cobra.Command{
		Use:   "get <draft-id>",
		Short: "Show a draft",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if bodyKind != "text" && bodyKind != "html" {
				return fmt.Errorf("gmail: --body must be text or html, got %q", bodyKind)
			}
			q := url.Values{"format": {"full"}}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/users/me/drafts/"+url.PathEscape(args[0]), q, nil)
			if err != nil {
				return err
			}
			var draft struct {
				ID      string  `json:"id"`
				Message *apiMsg `json:"message"`
			}
			if err := json.Unmarshal(body, &draft); err != nil {
				return fmt.Errorf("gmail: decode draft: %w", err)
			}
			if draft.Message == nil {
				return fmt.Errorf("gmail: draft %s has no message", args[0])
			}
			view, err := buildView(draft.Message, bodyKind, false)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitJSON(map[string]any{"id": draft.ID, "message": view})
			}
			fmt.Fprintf(s.stdout(), "Draft:   %s\n", draft.ID)
			renderMessage(s.stdout(), view)
			return nil
		},
	}
	cmd.Flags().StringVar(&bodyKind, "body", "text", "body variant to show: text or html")
	return cmd
}

func (s *Service) newDraftsSendCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "send <draft-id>",
		Short: "Send an existing draft",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]string{"id": args[0]}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/users/me/drafts/send", nil, payload)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				ID       string `json:"id"`
				ThreadID string `json:"threadId"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("gmail: decode send response: %w", err)
			}
			fmt.Fprintf(s.stdout(), "sent draft %s as message %s (thread %s)\n", args[0], resp.ID, resp.ThreadID)
			return nil
		},
	}
}

func (s *Service) newDraftsDeleteCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <draft-id>",
		Short: "Delete a draft",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := s.call(cmd.Context(), token, http.MethodDelete, "/users/me/drafts/"+url.PathEscape(args[0]), nil, nil); err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitJSON(map[string]string{"id": args[0], "status": "deleted"})
			}
			fmt.Fprintf(s.stdout(), "deleted draft %s\n", args[0])
			return nil
		},
	}
}

// emitDraft prints a drafts create/update response.
func (s *Service) emitDraft(cmd *cobra.Command, body []byte, verb string) error {
	if jsonOut(cmd) {
		return s.emit(body)
	}
	var draft struct {
		ID      string `json:"id"`
		Message struct {
			ID string `json:"id"`
		} `json:"message"`
	}
	if err := json.Unmarshal(body, &draft); err != nil {
		return fmt.Errorf("gmail: decode draft: %w", err)
	}
	fmt.Fprintf(s.stdout(), "%s %s (message %s)\n", verb, draft.ID, draft.Message.ID)
	return nil
}
