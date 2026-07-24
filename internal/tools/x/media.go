package x

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
)

const maxSimpleImageBytes = 5 << 20

// supportedSimpleImageTypes are the content types the one-shot simple upload
// API accepts; anything else (video, GIF, oversized images) goes chunked.
var supportedSimpleImageTypes = map[string]struct{}{
	"image/jpeg": {},
	"image/png":  {},
	"image/webp": {},
}

func (s *Service) newMediaCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "media", Short: "Media"}
	cmd.AddCommand(
		s.newMediaUploadCmd(token),
		s.newMediaStatusCmd(token),
		s.newMediaMetadataCmd(token),
	)
	return cmd
}

func (s *Service) newMediaUploadCmd(token string) *cobra.Command {
	var file, category string
	cmd := &cobra.Command{
		Use:         "upload",
		Short:       "Upload one image, GIF, or video and wait until it is ready to attach",
		Args:        cobra.NoArgs,
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireMediaCategory(category); err != nil {
				return err
			}
			info, err := os.Stat(file)
			if err != nil {
				return fmt.Errorf("read media file: %w", err)
			}
			if info.IsDir() {
				return fmt.Errorf("--file %q is a directory", file)
			}
			if info.Size() == 0 {
				return fmt.Errorf("--file %q is empty", file)
			}
			sniff, err := sniffMediaFile(file)
			if err != nil {
				return err
			}
			mediaType, defaultCategory, err := mediaTypeForUpload(sniff, file)
			if err != nil {
				return err
			}
			if category == "" {
				category = defaultCategory
			}
			var body []byte
			_, simple := supportedSimpleImageTypes[mediaType]
			if simple && info.Size() <= maxSimpleImageBytes && strings.HasSuffix(category, "_image") {
				body, err = s.simpleUpload(cmd.Context(), token, file, mediaType, category)
			} else {
				body, err = s.chunkedUpload(cmd.Context(), token, file, category)
			}
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "media file to upload (image, GIF, or video)")
	cmd.Flags().StringVar(&category, "category", "", "media use: tweet_image, tweet_video, tweet_gif, dm_image, dm_video, dm_gif, or amplify_video (empty = derived from the file type)")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func (s *Service) newMediaStatusCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "status <media-id>",
		Short:       "Get media upload processing status",
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireNumericID("media id", args[0]); err != nil {
				return err
			}
			query := url.Values{"media_id": {args[0]}, "command": {"STATUS"}}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/2/media/upload", query, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newMediaMetadataCmd(token string) *cobra.Command {
	var altText string
	cmd := &cobra.Command{
		Use:         "metadata <media-id>",
		Short:       "Set media alt text",
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireNumericID("media id", args[0]); err != nil {
				return err
			}
			if altText == "" {
				return fmt.Errorf("alt text must not be empty")
			}
			if utf8.RuneCountInString(altText) > 1000 {
				return fmt.Errorf("alt text must not exceed 1000 characters")
			}
			payload := struct {
				ID       string `json:"id"`
				Metadata struct {
					AltText struct {
						Text string `json:"text"`
					} `json:"alt_text"`
				} `json:"metadata"`
			}{ID: args[0]}
			payload.Metadata.AltText.Text = altText
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/2/media/metadata", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&altText, "alt-text", "", "accessible description (maximum 1000 characters)")
	_ = cmd.MarkFlagRequired("alt-text")
	return cmd
}

// simpleUpload posts one small JPEG, PNG, or WebP through the one-shot
// /2/media/upload endpoint and returns the response body.
func (s *Service) simpleUpload(ctx context.Context, token, file, mediaType, category string) ([]byte, error) {
	contents, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read media file: %w", err)
	}
	payload := struct {
		Media         string `json:"media"`
		MediaType     string `json:"media_type"`
		MediaCategory string `json:"media_category"`
	}{
		Media:         base64.StdEncoding.EncodeToString(contents),
		MediaType:     mediaType,
		MediaCategory: category,
	}
	return s.call(ctx, token, http.MethodPost, "/2/media/upload", nil, payload)
}
