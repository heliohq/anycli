package gmail

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/mail"
	"strings"

	"github.com/spf13/cobra"
)

func (s *Service) newMessagesSendCmd(token string) *cobra.Command {
	var o composeOptions
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send an email",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := o.resolveComposeBody()
			if err != nil {
				return err
			}
			raw, err := buildMIME(mimeMessage{
				to: o.to, cc: o.cc, bcc: o.bcc,
				subject: o.subject, body: body, html: o.html,
				attachments: o.attachments,
			})
			if err != nil {
				return err
			}
			return s.sendRaw(cmd, token, raw, "")
		},
	}
	addAddressFlags(cmd, &o)
	addBodyFlags(cmd, &o)
	return cmd
}

func (s *Service) newMessagesReplyCmd(token string) *cobra.Command {
	var o composeOptions
	var replyAll bool
	cmd := &cobra.Command{
		Use:   "reply <message-id>",
		Short: "Reply to a message (sender only; --all for reply-all)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := o.resolveComposeBody()
			if err != nil {
				return err
			}
			orig, err := s.fetchMessage(cmd.Context(), token, args[0])
			if err != nil {
				return err
			}
			to, cc, err := s.replyRecipients(cmd.Context(), token, orig, replyAll)
			if err != nil {
				return err
			}
			inReplyTo := orig.header("Message-ID")
			raw, err := buildMIME(mimeMessage{
				to:          to,
				cc:          cc,
				subject:     replySubject(orig.header("Subject")),
				inReplyTo:   inReplyTo,
				references:  joinReferences(orig.header("References"), inReplyTo),
				body:        body,
				html:        o.html,
				attachments: o.attachments,
			})
			if err != nil {
				return err
			}
			return s.sendRaw(cmd, token, raw, orig.ThreadID)
		},
	}
	addBodyFlags(cmd, &o)
	cmd.Flags().BoolVar(&replyAll, "all", false, "reply to all original recipients")
	return cmd
}

func (s *Service) newMessagesForwardCmd(token string) *cobra.Command {
	var to []string
	var preamble string
	cmd := &cobra.Command{
		Use:   "forward <message-id>",
		Short: "Forward a message with the original quoted",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			orig, err := s.fetchMessage(cmd.Context(), token, args[0])
			if err != nil {
				return err
			}
			body, err := forwardBody(preamble, orig)
			if err != nil {
				return err
			}
			raw, err := buildMIME(mimeMessage{
				to:      to,
				subject: forwardSubject(orig.header("Subject")),
				body:    body,
			})
			if err != nil {
				return err
			}
			return s.sendRaw(cmd, token, raw, "")
		},
	}
	cmd.Flags().StringSliceVar(&to, "to", nil, "recipient addresses (comma-separated or repeated)")
	cmd.Flags().StringVar(&preamble, "body", "", "optional preamble above the quoted message")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}

// sendRaw posts an assembled MIME message to messages.send, optionally
// threading it (reply), and emits the API response.
func (s *Service) sendRaw(cmd *cobra.Command, token string, raw []byte, threadID string) error {
	payload := map[string]string{"raw": rawField(raw)}
	if threadID != "" {
		payload["threadId"] = threadID
	}
	body, err := s.call(cmd.Context(), token, http.MethodPost, "/users/me/messages/send", nil, payload)
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
	fmt.Fprintf(s.stdout(), "sent message %s (thread %s)\n", resp.ID, resp.ThreadID)
	return nil
}

// replyRecipients derives To/Cc for a reply. Default: sender only (Reply-To,
// falling back to From). With replyAll, the remaining original To + Cc
// entries land on Cc, minus the reply target and the connected mailbox
// itself (resolved via users.getProfile).
func (s *Service) replyRecipients(ctx context.Context, token string, orig *apiMsg, replyAll bool) (to, cc []string, err error) {
	target := orig.header("Reply-To")
	if target == "" {
		target = orig.header("From")
	}
	if target == "" {
		return nil, nil, fmt.Errorf("gmail: message %s has no Reply-To or From header to reply to", orig.ID)
	}
	to = []string{target}
	if !replyAll {
		return to, nil, nil
	}

	self, err := s.profileEmail(ctx, token)
	if err != nil {
		return nil, nil, err
	}
	exclude := map[string]bool{strings.ToLower(self): true}
	targetAddrs, err := parseAddressList(target)
	if err != nil {
		return nil, nil, fmt.Errorf("gmail: parse reply target %q: %w", target, err)
	}
	for _, a := range targetAddrs {
		exclude[strings.ToLower(a.Address)] = true
	}
	for _, name := range []string{"To", "Cc"} {
		header := orig.header(name)
		if header == "" {
			continue
		}
		addrs, err := parseAddressList(header)
		if err != nil {
			return nil, nil, fmt.Errorf("gmail: parse original %s header %q: %w", name, header, err)
		}
		for _, a := range addrs {
			key := strings.ToLower(a.Address)
			if exclude[key] {
				continue
			}
			exclude[key] = true
			cc = append(cc, a.String())
		}
	}
	return to, cc, nil
}

func parseAddressList(header string) ([]*mail.Address, error) {
	return mail.ParseAddressList(header)
}

// profileEmail resolves the connected mailbox address.
func (s *Service) profileEmail(ctx context.Context, token string) (string, error) {
	body, err := s.call(ctx, token, http.MethodGet, "/users/me/profile", nil, nil)
	if err != nil {
		return "", err
	}
	var p struct {
		EmailAddress string `json:"emailAddress"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return "", fmt.Errorf("gmail: decode profile: %w", err)
	}
	if p.EmailAddress == "" {
		return "", fmt.Errorf("gmail: profile has no emailAddress")
	}
	return p.EmailAddress, nil
}

// replySubject prefixes Re: unless the subject already carries it.
func replySubject(subject string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(subject)), "re:") {
		return subject
	}
	return "Re: " + subject
}

// forwardSubject prefixes Fwd: unless the subject already carries it.
func forwardSubject(subject string) string {
	trimmed := strings.ToLower(strings.TrimSpace(subject))
	if strings.HasPrefix(trimmed, "fwd:") || strings.HasPrefix(trimmed, "fw:") {
		return subject
	}
	return "Fwd: " + subject
}

// joinReferences appends the replied-to Message-ID to the original References
// chain, per RFC 5322 threading.
func joinReferences(references, messageID string) string {
	return strings.TrimSpace(references + " " + messageID)
}

// forwardBody composes the forward text: optional preamble, then the standard
// quoted-original block.
func forwardBody(preamble string, orig *apiMsg) (string, error) {
	origBody, _, err := orig.resolveBody("text")
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if preamble != "" {
		b.WriteString(preamble)
		b.WriteString("\n\n")
	}
	b.WriteString("---------- Forwarded message ---------\n")
	for _, h := range []string{"From", "Date", "Subject", "To", "Cc"} {
		if v := orig.header(h); v != "" {
			fmt.Fprintf(&b, "%s: %s\n", h, v)
		}
	}
	b.WriteString("\n")
	b.WriteString(origBody)
	return b.String(), nil
}
