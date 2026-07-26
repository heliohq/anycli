package tiktok

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

func (s *Service) newVideoCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "video", Short: "Read the creator's videos"}
	cmd.AddCommand(
		s.newVideoListCmd(token),
		s.newVideoQueryCmd(token),
	)
	return cmd
}

func (s *Service) newVideoListCmd(token string) *cobra.Command {
	var fields string
	var cursor int64
	var maxCount int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the creator's videos (one page)",
		Long: "`--max-count` is 1-20, default 10, and anything outside that range is\n" +
			"rejected before a request goes out. `--cursor` is a UTC unix MILLISECOND\n" +
			"timestamp, not an opaque token: pass the `cursor` from the previous page to\n" +
			"continue, and `has_more` says whether continuing is worthwhile. `--fields`\n" +
			"already asks for the engagement counters (`like_count`, `comment_count`,\n" +
			"`share_count`, `view_count`) alongside the metadata, so a per-video stats\n" +
			"pass needs no second call.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireRange("--max-count", maxCount, 1, 20); err != nil {
				return err
			}
			body := map[string]any{"max_count": maxCount}
			if cursor > 0 {
				body["cursor"] = cursor
			}
			query := url.Values{"fields": {fields}}
			data, err := s.call(cmd.Context(), token, http.MethodPost, "/v2/video/list/", query, body)
			if err != nil {
				return err
			}
			return s.emit(data)
		},
	}
	cmd.Flags().StringVar(&fields, "fields", defaultVideoFields, "comma-separated video fields to return")
	cmd.Flags().Int64Var(&cursor, "cursor", 0, "pagination cursor (UTC unix ms) from a previous page")
	cmd.Flags().IntVar(&maxCount, "max-count", 10, "videos per page (1-20)")
	return cmd
}

func (s *Service) newVideoQueryCmd(token string) *cobra.Command {
	var fields, ids string
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Query specific videos by id",
		Long: "`--ids` is required and comma-separated, and the ids come from `video list`\n" +
			"— there is no lookup by URL, caption or share link. Only videos belonging\n" +
			"to the connected creator resolve. Use this to refresh counters on videos\n" +
			"already known instead of paging `video list` again.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			videoIDs := splitCSV(ids)
			if len(videoIDs) == 0 {
				return errRequired("--ids")
			}
			body := map[string]any{
				"filters": map[string]any{"video_ids": videoIDs},
			}
			query := url.Values{"fields": {fields}}
			data, err := s.call(cmd.Context(), token, http.MethodPost, "/v2/video/query/", query, body)
			if err != nil {
				return err
			}
			return s.emit(data)
		},
	}
	cmd.Flags().StringVar(&fields, "fields", defaultVideoFields, "comma-separated video fields to return")
	cmd.Flags().StringVar(&ids, "ids", "", "comma-separated video ids (required)")
	return cmd
}

func splitCSV(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
