package gmail

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"mime"
	"mime/multipart"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// maxMessageBytes is Gmail's outbound message size limit (25MB).
const maxMessageBytes = 25 << 20

// composeOptions carries the shared send / reply / drafts flag values.
type composeOptions struct {
	to          []string
	cc          []string
	bcc         []string
	subject     string
	body        string
	bodyFile    string
	html        bool
	attachments []string
}

// addBodyFlags wires --body / --body-file / --html / --attach on a command.
// Exactly one of --body / --body-file is required.
func addBodyFlags(cmd *cobra.Command, o *composeOptions) {
	cmd.Flags().StringVar(&o.body, "body", "", "message body")
	cmd.Flags().StringVar(&o.bodyFile, "body-file", "", "read the message body from a file")
	cmd.Flags().BoolVar(&o.html, "html", false, "send the body as text/html")
	cmd.Flags().StringArrayVar(&o.attachments, "attach", nil, "attach a file (repeatable)")
	cmd.MarkFlagsOneRequired("body", "body-file")
	cmd.MarkFlagsMutuallyExclusive("body", "body-file")
}

// addAddressFlags wires --to / --cc / --bcc / --subject on a command.
func addAddressFlags(cmd *cobra.Command, o *composeOptions) {
	cmd.Flags().StringSliceVar(&o.to, "to", nil, "recipient addresses (comma-separated or repeated)")
	cmd.Flags().StringSliceVar(&o.cc, "cc", nil, "Cc addresses")
	cmd.Flags().StringSliceVar(&o.bcc, "bcc", nil, "Bcc addresses")
	cmd.Flags().StringVar(&o.subject, "subject", "", "subject line")
	_ = cmd.MarkFlagRequired("to")
	_ = cmd.MarkFlagRequired("subject")
}

// resolveComposeBody returns the body text from --body or --body-file.
func (o *composeOptions) resolveComposeBody() (string, error) {
	if o.bodyFile == "" {
		return o.body, nil
	}
	data, err := os.ReadFile(o.bodyFile)
	if err != nil {
		return "", fmt.Errorf("gmail: read body file: %w", err)
	}
	return string(data), nil
}

// mimeMessage is the input to buildMIME. Gmail sets From itself.
type mimeMessage struct {
	to          []string
	cc          []string
	bcc         []string
	subject     string
	inReplyTo   string
	references  string
	body        string
	html        bool
	attachments []string
}

// buildMIME assembles the RFC 822 message Gmail's send/draft APIs expect in
// the raw field: single-part for a bare body, multipart/mixed with base64
// attachment parts otherwise. Messages over 25MB are rejected.
func buildMIME(m mimeMessage) ([]byte, error) {
	var buf bytes.Buffer
	writeHeader := func(name, value string) {
		if value != "" {
			fmt.Fprintf(&buf, "%s: %s\r\n", name, value)
		}
	}
	writeHeader("To", strings.Join(m.to, ", "))
	writeHeader("Cc", strings.Join(m.cc, ", "))
	writeHeader("Bcc", strings.Join(m.bcc, ", "))
	writeHeader("Subject", mime.QEncoding.Encode("UTF-8", m.subject))
	writeHeader("In-Reply-To", m.inReplyTo)
	writeHeader("References", m.references)
	writeHeader("MIME-Version", "1.0")

	textType := "text/plain"
	if m.html {
		textType = "text/html"
	}
	if len(m.attachments) == 0 {
		fmt.Fprintf(&buf, "Content-Type: %s; charset=\"UTF-8\"\r\n\r\n", textType)
		buf.WriteString(m.body)
	} else if err := writeMultipart(&buf, textType, m.body, m.attachments); err != nil {
		return nil, err
	}
	if buf.Len() > maxMessageBytes {
		return nil, fmt.Errorf("gmail: message is %d bytes; the Gmail limit is 25MB", buf.Len())
	}
	return buf.Bytes(), nil
}

func writeMultipart(buf *bytes.Buffer, textType, body string, attachments []string) error {
	w := multipart.NewWriter(buf)
	fmt.Fprintf(buf, "Content-Type: multipart/mixed; boundary=%q\r\n\r\n", w.Boundary())

	textPart, err := w.CreatePart(textproto.MIMEHeader{
		"Content-Type": {textType + `; charset="UTF-8"`},
	})
	if err != nil {
		return fmt.Errorf("gmail: build text part: %w", err)
	}
	if _, err := textPart.Write([]byte(body)); err != nil {
		return fmt.Errorf("gmail: write text part: %w", err)
	}

	for _, path := range attachments {
		if err := writeAttachmentPart(w, path); err != nil {
			return err
		}
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("gmail: finalize multipart message: %w", err)
	}
	return nil
}

func writeAttachmentPart(w *multipart.Writer, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("gmail: read attachment %s: %w", path, err)
	}
	name := filepath.Base(path)
	contentType := mime.TypeByExtension(filepath.Ext(path))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	part, err := w.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {contentType},
		"Content-Disposition":       {fmt.Sprintf("attachment; filename=%q", name)},
		"Content-Transfer-Encoding": {"base64"},
	})
	if err != nil {
		return fmt.Errorf("gmail: build attachment part: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	for len(encoded) > 0 {
		line := encoded
		if len(line) > 76 {
			line = line[:76]
		}
		encoded = encoded[len(line):]
		if _, err := part.Write([]byte(line + "\r\n")); err != nil {
			return fmt.Errorf("gmail: write attachment part: %w", err)
		}
	}
	return nil
}

// rawField encodes an assembled MIME message for the API's raw field.
func rawField(message []byte) string {
	return base64.URLEncoding.EncodeToString(message)
}
