package hootsuite

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newMediaCreateCmd requests a media upload URL. The caller PUTs bytes to the
// returned uploadUrl, then references the returned id in a message's media.
func (s *Service) newMediaCreateCmd(token string) *cobra.Command {
	var (
		sizeBytes int64
		mimeType  string
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Request a media upload URL (POST /v1/media)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if sizeBytes <= 0 {
				return &usageError{msg: "--size-bytes must be a positive integer"}
			}
			if mimeType == "" {
				return &usageError{msg: "--mime-type is required"}
			}
			payload := map[string]any{"sizeBytes": sizeBytes, "mimeType": mimeType}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/media", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().Int64Var(&sizeBytes, "size-bytes", 0, "size of the media in bytes (required)")
	cmd.Flags().StringVar(&mimeType, "mime-type", "", "media MIME type, e.g. image/png (required)")
	return cmd
}

// newMediaGetCmd polls a media upload's processing status (READY before use).
func (s *Service) newMediaGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get media upload status (GET /v1/media/{id})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/media/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}
