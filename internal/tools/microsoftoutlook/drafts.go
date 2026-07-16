package microsoftoutlook

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func (s *Service) newDraftsCreateCmd(token string) *cobra.Command {
	var o composeOptions
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a draft (same parameters as messages send)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			bodyText, err := o.resolveComposeBody()
			if err != nil {
				return err
			}
			msg, err := buildGraphMessage(&o, bodyText)
			if err != nil {
				return err
			}
			// POST /me/messages with a message body creates a draft (isDraft).
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/me/messages", nil, msg)
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
			bodyText, err := o.resolveComposeBody()
			if err != nil {
				return err
			}
			msg, err := buildGraphMessage(&o, bodyText)
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodPatch, "/me/messages/"+url.PathEscape(args[0]), nil, msg)
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
	var page string
	var max int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List drafts (GET /me/mailFolders/drafts/messages)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var body []byte
			var err error
			if page != "" {
				body, err = s.call(cmd.Context(), token, http.MethodGet, page, nil, nil)
			} else {
				q := url.Values{}
				q.Set("$top", strconv.Itoa(max))
				body, err = s.call(cmd.Context(), token, http.MethodGet, "/me/mailFolders/drafts/messages", q, nil)
			}
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				Value    []graphMessage `json:"value"`
				NextLink string         `json:"@odata.nextLink"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("microsoft-outlook: decode draft list: %w", err)
			}
			if len(resp.Value) == 0 {
				fmt.Fprintln(s.stdout(), "no drafts")
				return nil
			}
			for _, d := range resp.Value {
				fmt.Fprintf(s.stdout(), "%s\t%s\t%s\n", d.ID, formatRecipients(d.ToRecipients), d.Subject)
			}
			if resp.NextLink != "" {
				fmt.Fprintf(s.stdout(), "next page: %s\n", resp.NextLink)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&max, "max", 10, "max results to return ($top)")
	cmd.Flags().StringVar(&page, "page", "", "@odata.nextLink from a previous list call")
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
				return fmt.Errorf("microsoft-outlook: --body must be text or html, got %q", bodyKind)
			}
			m, err := s.fetchMessage(cmd.Context(), token, args[0], bodyKind, false)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitJSON(m)
			}
			fmt.Fprintf(s.stdout(), "Draft:    %s\n", m.ID)
			s.renderMessage(s.stdout(), m)
			return nil
		},
	}
	cmd.Flags().StringVar(&bodyKind, "body", "text", "body variant to show: text or html")
	return cmd
}

func (s *Service) newDraftsSendCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "send <draft-id>",
		Short: "Send an existing draft (POST /me/messages/{id}/send)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.sendDraft(cmd, token, args[0], "sent draft")
		},
	}
}

func (s *Service) newDraftsDeleteCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <draft-id>",
		Short: "Delete a draft (DELETE /me/messages/{id})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := s.call(cmd.Context(), token, http.MethodDelete, "/me/messages/"+url.PathEscape(args[0]), nil, nil); err != nil {
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
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &draft); err != nil {
		return fmt.Errorf("microsoft-outlook: decode draft: %w", err)
	}
	fmt.Fprintf(s.stdout(), "%s %s\n", verb, draft.ID)
	return nil
}
