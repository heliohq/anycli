package gmail

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// savedAttachment is one downloaded attachment (--json contract).
type savedAttachment struct {
	AttachmentID string `json:"attachmentId"`
	Filename     string `json:"filename"`
	Path         string `json:"path"`
	Size         int    `json:"size"`
}

func (s *Service) newMessagesAttachmentsCmd(token string) *cobra.Command {
	var names []string
	var indexes []int
	var saveDir string
	cmd := &cobra.Command{
		Use:   "attachments <message-id>",
		Short: "Download message attachments (all parts by default; select with --name / --index)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			messageID := args[0]
			// Fetch the message fresh and download with the attachmentIds
			// from this same response: Gmail regenerates attachmentIds on
			// every messages.get, so ids from an earlier get are often
			// invalid.
			m, err := s.fetchMessage(cmd.Context(), token, messageID)
			if err != nil {
				return err
			}
			inventory, err := selectAttachments(m.attachments(), names, indexes)
			if err != nil {
				return fmt.Errorf("gmail: message %s: %w", messageID, err)
			}
			if len(inventory) == 0 {
				if jsonOut(cmd) {
					return s.emitJSON([]savedAttachment{})
				}
				fmt.Fprintln(s.stdout(), "no attachments")
				return nil
			}
			if err := os.MkdirAll(saveDir, 0o755); err != nil {
				return fmt.Errorf("gmail: create save dir: %w", err)
			}
			saved, err := s.downloadAttachments(cmd.Context(), token, messageID, inventory, saveDir)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitJSON(saved)
			}
			for _, a := range saved {
				fmt.Fprintf(s.stdout(), "saved %s (%d bytes)\n", a.Path, a.Size)
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&names, "name", nil, "download only attachments with this exact filename (repeatable)")
	cmd.Flags().IntSliceVar(&indexes, "index", nil, "download only the n-th attachment, 1-based as listed by `messages get` (repeatable)")
	cmd.Flags().StringVar(&saveDir, "save", ".", "directory to save attachments into")
	return cmd
}

// selectAttachments filters the attachment inventory by --name (exact
// filename match) and --index (1-based position in the inventory as printed
// by `messages get`). No selectors = the whole inventory. Every selector
// must match at least one part.
func selectAttachments(inventory []attachmentInfo, names []string, indexes []int) ([]attachmentInfo, error) {
	if len(names) == 0 && len(indexes) == 0 {
		return inventory, nil
	}
	take := make([]bool, len(inventory))
	for _, name := range names {
		found := false
		for i, a := range inventory {
			if a.Filename == name {
				take[i] = true
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("no attachment named %q", name)
		}
	}
	for _, idx := range indexes {
		if idx < 1 || idx > len(inventory) {
			return nil, fmt.Errorf("attachment index %d out of range (message has %d attachments)", idx, len(inventory))
		}
		take[idx-1] = true
	}
	selected := make([]attachmentInfo, 0, len(inventory))
	for i, a := range inventory {
		if take[i] {
			selected = append(selected, a)
		}
	}
	return selected, nil
}

func (s *Service) downloadAttachments(ctx context.Context, token, messageID string, inventory []attachmentInfo, saveDir string) ([]savedAttachment, error) {
	saved := make([]savedAttachment, 0, len(inventory))
	used := map[string]int{}
	for _, att := range inventory {
		body, err := s.call(ctx, token, http.MethodGet,
			"/users/me/messages/"+url.PathEscape(messageID)+"/attachments/"+url.PathEscape(att.AttachmentID), nil, nil)
		if err != nil {
			return nil, err
		}
		var resp struct {
			Data string `json:"data"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("gmail: decode attachment %s: %w", att.AttachmentID, err)
		}
		data, err := decodeBase64URL(resp.Data)
		if err != nil {
			return nil, err
		}
		name := uniqueName(used, attachmentFilename(att))
		path := filepath.Join(saveDir, name)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return nil, fmt.Errorf("gmail: write attachment %s: %w", name, err)
		}
		saved = append(saved, savedAttachment{
			AttachmentID: att.AttachmentID,
			Filename:     name,
			Path:         path,
			Size:         len(data),
		})
	}
	return saved, nil
}

// attachmentFilename picks a safe on-disk name for one attachment.
func attachmentFilename(att attachmentInfo) string {
	name := filepath.Base(strings.TrimSpace(att.Filename))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "attachment-" + att.PartID
	}
	return name
}

// uniqueName dedupes filenames within one download run: b.txt, b-1.txt, ...
func uniqueName(used map[string]int, name string) string {
	n := used[name]
	used[name] = n + 1
	if n == 0 {
		return name
	}
	ext := filepath.Ext(name)
	return fmt.Sprintf("%s-%d%s", strings.TrimSuffix(name, ext), n, ext)
}
