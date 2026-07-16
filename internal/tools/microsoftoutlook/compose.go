package microsoftoutlook

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// maxMessageBytes is Graph's practical inline-attachment ceiling. Larger
// attachments require an upload session (not implemented for mail v1).
const maxMessageBytes = 3 << 20

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
	cmd.Flags().BoolVar(&o.html, "html", false, "send the body as HTML")
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
		return "", fmt.Errorf("microsoft-outlook: read body file: %w", err)
	}
	return string(data), nil
}

// recipients builds Graph's recipient array from a list of addresses.
func recipients(addrs []string) []map[string]any {
	out := make([]map[string]any, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, map[string]any{"emailAddress": map[string]any{"address": a}})
	}
	return out
}

// buildGraphMessage assembles a Graph message resource from compose options.
// bodyText overrides o.body/o.bodyFile (already resolved by the caller).
func buildGraphMessage(o *composeOptions, bodyText string) (map[string]any, error) {
	contentType := "text"
	if o.html {
		contentType = "html"
	}
	msg := map[string]any{
		"subject": o.subject,
		"body": map[string]any{
			"contentType": contentType,
			"content":     bodyText,
		},
	}
	if len(o.to) > 0 {
		msg["toRecipients"] = recipients(o.to)
	}
	if len(o.cc) > 0 {
		msg["ccRecipients"] = recipients(o.cc)
	}
	if len(o.bcc) > 0 {
		msg["bccRecipients"] = recipients(o.bcc)
	}
	if len(o.attachments) > 0 {
		atts, err := fileAttachments(o.attachments)
		if err != nil {
			return nil, err
		}
		msg["attachments"] = atts
	}
	return msg, nil
}

// fileAttachments reads local files into Graph fileAttachment objects
// (base64-encoded contentBytes). Total size is capped at maxMessageBytes.
func fileAttachments(paths []string) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(paths))
	var total int
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("microsoft-outlook: read attachment %s: %w", path, err)
		}
		total += len(data)
		if total > maxMessageBytes {
			return nil, fmt.Errorf("microsoft-outlook: attachments exceed %d bytes (use OneDrive for large files)", maxMessageBytes)
		}
		out = append(out, map[string]any{
			"@odata.type":  "#microsoft.graph.fileAttachment",
			"name":         filepath.Base(path),
			"contentBytes": base64.StdEncoding.EncodeToString(data),
		})
	}
	return out, nil
}
