package postmark

import (
	"encoding/base64"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// commonSendFlags holds the address, tracking, and metadata fields shared by
// `email send` and `email send-template`. Only fields the caller set are
// written into the request body so Postmark applies its own server defaults for
// the rest (e.g. MessageStream defaults to "outbound").
type commonSendFlags struct {
	from        string
	to          string
	cc          string
	bcc         string
	replyTo     string
	tag         string
	stream      string
	trackOpens  bool
	trackLinks  string
	metadata    string
	headers     []string
	attachments []string
}

func (f *commonSendFlags) register(cmd *cobra.Command) {
	fl := cmd.Flags()
	fl.StringVar(&f.from, "from", "", "sender address (must be a confirmed Sender Signature) [required]")
	fl.StringVar(&f.to, "to", "", "recipient address(es), comma-separated [required]")
	fl.StringVar(&f.cc, "cc", "", "cc address(es), comma-separated")
	fl.StringVar(&f.bcc, "bcc", "", "bcc address(es), comma-separated")
	fl.StringVar(&f.replyTo, "reply-to", "", "Reply-To address")
	fl.StringVar(&f.tag, "tag", "", "message tag")
	fl.StringVar(&f.stream, "stream", "", "message stream id (default: outbound)")
	fl.BoolVar(&f.trackOpens, "track-opens", false, "enable open tracking")
	fl.StringVar(&f.trackLinks, "track-links", "", "link tracking: None|HtmlAndText|HtmlOnly|TextOnly")
	fl.StringVar(&f.metadata, "metadata", "", "custom metadata as a JSON object")
	fl.StringArrayVar(&f.headers, "header", nil, "custom header as 'Name: Value' (repeatable)")
	fl.StringArrayVar(&f.attachments, "attachment", nil, "file path to attach (repeatable)")
}

// applyTo writes the shared fields into a Postmark request body map. It
// validates required and enum fields, returning a usageError (exit 2) on bad
// input.
func (f *commonSendFlags) applyTo(body map[string]any) error {
	if strings.TrimSpace(f.from) == "" {
		return usagef("postmark: --from is required")
	}
	if strings.TrimSpace(f.to) == "" {
		return usagef("postmark: --to is required")
	}
	body["From"] = f.from
	body["To"] = f.to
	setIf(body, "Cc", f.cc)
	setIf(body, "Bcc", f.bcc)
	setIf(body, "ReplyTo", f.replyTo)
	setIf(body, "Tag", f.tag)
	setIf(body, "MessageStream", f.stream)
	if f.trackOpens {
		body["TrackOpens"] = true
	}
	if f.trackLinks != "" {
		canonical, ok := canonicalTrackLinks(f.trackLinks)
		if !ok {
			return usagef("postmark: --track-links must be one of None|HtmlAndText|HtmlOnly|TextOnly, got %q", f.trackLinks)
		}
		body["TrackLinks"] = canonical
	}
	if f.metadata != "" {
		meta, err := decodeJSONObject("metadata", f.metadata)
		if err != nil {
			return err
		}
		body["Metadata"] = meta
	}
	if len(f.headers) > 0 {
		headers, err := parseHeaders(f.headers)
		if err != nil {
			return err
		}
		body["Headers"] = headers
	}
	if len(f.attachments) > 0 {
		attachments, err := readAttachments(f.attachments)
		if err != nil {
			return err
		}
		body["Attachments"] = attachments
	}
	return nil
}

func (s *Service) newEmailCmd(token string) *cobra.Command {
	group := newGroupCmd("email", "Send email")
	group.AddCommand(s.newEmailSendCmd(token), s.newEmailSendTemplateCmd(token))
	return group
}

func (s *Service) newEmailSendCmd(token string) *cobra.Command {
	var common commonSendFlags
	var subject, html, text string
	cmd := &cobra.Command{
		Use:         "send",
		Short:       "Send a single email (POST /email)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{}
			if err := common.applyTo(body); err != nil {
				return err
			}
			setIf(body, "Subject", subject)
			setIf(body, "HtmlBody", html)
			setIf(body, "TextBody", text)
			if html == "" && text == "" {
				return usagef("postmark: provide at least one of --html or --text")
			}
			raw, err := s.call(cmd.Context(), token, http.MethodPost, "/email", nil, body)
			if err != nil {
				return err
			}
			return s.emit(raw)
		},
	}
	common.register(cmd)
	cmd.Flags().StringVar(&subject, "subject", "", "email subject")
	cmd.Flags().StringVar(&html, "html", "", "HTML body")
	cmd.Flags().StringVar(&text, "text", "", "plain-text body")
	return cmd
}

func (s *Service) newEmailSendTemplateCmd(token string) *cobra.Command {
	var common commonSendFlags
	var templateID int
	var templateAlias, model string
	cmd := &cobra.Command{
		Use:         "send-template",
		Short:       "Send using a template (POST /email/withTemplate)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			hasID := cmd.Flags().Changed("template-id")
			if hasID == (templateAlias != "") {
				return usagef("postmark: provide exactly one of --template-id or --template-alias")
			}
			body := map[string]any{}
			if err := common.applyTo(body); err != nil {
				return err
			}
			if hasID {
				body["TemplateId"] = templateID
			} else {
				body["TemplateAlias"] = templateAlias
			}
			if model != "" {
				m, err := decodeJSONObject("model", model)
				if err != nil {
					return err
				}
				body["TemplateModel"] = m
			}
			raw, err := s.call(cmd.Context(), token, http.MethodPost, "/email/withTemplate", nil, body)
			if err != nil {
				return err
			}
			return s.emit(raw)
		},
	}
	common.register(cmd)
	cmd.Flags().IntVar(&templateID, "template-id", 0, "numeric template id")
	cmd.Flags().StringVar(&templateAlias, "template-alias", "", "template alias")
	cmd.Flags().StringVar(&model, "model", "", "TemplateModel as a JSON object")
	return cmd
}

// setIf writes key=value into body only when value is non-empty.
func setIf(body map[string]any, key, value string) {
	if value != "" {
		body[key] = value
	}
}

// canonicalTrackLinks maps a case-insensitive --track-links value to Postmark's
// canonical enum spelling.
func canonicalTrackLinks(value string) (string, bool) {
	for _, canonical := range []string{"None", "HtmlAndText", "HtmlOnly", "TextOnly"} {
		if strings.EqualFold(value, canonical) {
			return canonical, true
		}
	}
	return "", false
}

// parseHeaders converts "Name: Value" flag entries into Postmark's
// [{"Name","Value"}] header array.
func parseHeaders(entries []string) ([]map[string]string, error) {
	headers := make([]map[string]string, 0, len(entries))
	for _, entry := range entries {
		name, value, ok := strings.Cut(entry, ":")
		if !ok || strings.TrimSpace(name) == "" {
			return nil, usagef("postmark: --header %q must be in 'Name: Value' form", entry)
		}
		headers = append(headers, map[string]string{
			"Name":  strings.TrimSpace(name),
			"Value": strings.TrimSpace(value),
		})
	}
	return headers, nil
}

// readAttachments reads each file path, base64-encodes its bytes, and builds
// Postmark's [{"Name","Content","ContentType"}] attachment array. ContentType
// is inferred from the file extension, defaulting to application/octet-stream.
func readAttachments(paths []string) ([]map[string]string, error) {
	attachments := make([]map[string]string, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, usagef("postmark: cannot read attachment %q: %v", path, err)
		}
		contentType := mime.TypeByExtension(filepath.Ext(path))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		attachments = append(attachments, map[string]string{
			"Name":        filepath.Base(path),
			"Content":     base64.StdEncoding.EncodeToString(data),
			"ContentType": contentType,
		})
	}
	return attachments, nil
}
