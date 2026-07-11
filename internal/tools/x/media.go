package x

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"unicode/utf8"

	"github.com/spf13/cobra"
)

const maxSimpleImageBytes = 5 << 20

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
		Use:   "upload",
		Short: "Upload one JPEG, PNG, or WebP image with the simple upload API",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if category != "tweet_image" && category != "dm_image" {
				return fmt.Errorf("category must be tweet_image or dm_image")
			}
			contents, mediaType, err := readSimpleImage(file)
			if err != nil {
				return err
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
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/2/media/upload", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "image file to upload (maximum 5 MiB)")
	cmd.Flags().StringVar(&category, "category", "tweet_image", "media use: tweet_image or dm_image")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func (s *Service) newMediaStatusCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "status <media-id>",
		Short: "Get media upload processing status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireNumericID("media id", args[0]); err != nil {
				return err
			}
			query := url.Values{"media_id": {args[0]}}
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
		Use:   "metadata <media-id>",
		Short: "Set media alt text",
		Args:  cobra.ExactArgs(1),
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

func readSimpleImage(path string) ([]byte, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, "", fmt.Errorf("open media file: %w", err)
	}
	defer file.Close()

	contents, err := io.ReadAll(io.LimitReader(file, maxSimpleImageBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("read media file: %w", err)
	}
	if len(contents) > maxSimpleImageBytes {
		return nil, "", fmt.Errorf("media file exceeds the 5 MiB simple-image limit; chunked video/GIF upload is not supported yet")
	}
	mediaType := http.DetectContentType(contents)
	if _, ok := supportedSimpleImageTypes[mediaType]; !ok {
		return nil, "", fmt.Errorf("only JPEG, PNG, and WebP images are supported by simple upload; video/GIF upload is not supported yet")
	}
	return contents, mediaType, nil
}
