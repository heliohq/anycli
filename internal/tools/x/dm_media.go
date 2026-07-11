package x

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

var (
	dmMediaKeyPattern   = regexp.MustCompile(`^[0-9]+_[0-9]+$`)
	dmResourceIDPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,50}$`)
)

func (s *Service) newDMMediaCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "media", Short: "Legacy DM media"}
	cmd.AddCommand(s.newDMMediaDownloadCmd(token))
	return cmd
}

func (s *Service) newDMMediaDownloadCmd(token string) *cobra.Command {
	var eventID, mediaKey, resourceID, output string
	cmd := &cobra.Command{
		Use:   "download",
		Short: "Download legacy DM media as raw bytes to a file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireNumericID("DM event id", eventID); err != nil {
				return err
			}
			if !dmMediaKeyPattern.MatchString(mediaKey) {
				return fmt.Errorf("media key must have the numeric prefix_numeric-id format returned by DM expansions")
			}
			_, mediaID, _ := strings.Cut(mediaKey, "_")
			if err := requireNumericID("media id", mediaID); err != nil {
				return err
			}
			if !dmResourceIDPattern.MatchString(resourceID) || resourceID == "." || resourceID == ".." {
				return fmt.Errorf("resource id must contain 1-50 letters, numbers, dots, underscores, or hyphens")
			}
			if output == "" {
				return fmt.Errorf("output path must not be empty")
			}
			path := "/2/dm_conversations/media/" + url.PathEscape(eventID) + "/" + url.PathEscape(mediaID) + "/" + url.PathEscape(resourceID)
			written, err := s.download(cmd.Context(), token, path, output)
			if err != nil {
				return err
			}
			return s.emitValue(struct {
				Path  string `json:"path"`
				Bytes int64  `json:"bytes"`
			}{Path: output, Bytes: written})
		},
	}
	cmd.Flags().StringVar(&eventID, "event-id", "", "DM event id containing the media")
	cmd.Flags().StringVar(&mediaKey, "media-key", "", "media key returned by attachments.media_keys expansion")
	cmd.Flags().StringVar(&resourceID, "resource-id", "", "final path segment from the expanded media URL, including extension")
	cmd.Flags().StringVar(&output, "output", "", "destination file for raw binary bytes")
	_ = cmd.MarkFlagRequired("event-id")
	_ = cmd.MarkFlagRequired("media-key")
	_ = cmd.MarkFlagRequired("resource-id")
	_ = cmd.MarkFlagRequired("output")
	return cmd
}
