package typefully

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newDraftCmd groups the draft lifecycle — create/schedule/publish, list,
// read, edit, delete. This is the core of the tool.
func (s *Service) newDraftCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "draft", Short: "Create, schedule/publish, list, and manage drafts"}
	cmd.AddCommand(
		s.newDraftListCmd(token),
		s.newDraftGetCmd(token),
		s.newDraftCreateCmd(token),
		s.newDraftUpdateCmd(token),
		s.newDraftDeleteCmd(token),
	)
	return cmd
}

func (s *Service) newDraftListCmd(token string) *cobra.Command {
	var socialSet, status, tag, orderBy string
	var limit, offset int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List/filter drafts (GET /v2/social-sets/{id}/drafts)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if status != "" {
				q.Set("status", status)
			}
			if tag != "" {
				q.Set("tag", tag)
			}
			if orderBy != "" {
				q.Set("order_by", orderBy)
			}
			addPaging(q, limit, offset)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, scopedPath(socialSet, "/drafts"), q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	addSocialSetFlag(cmd, &socialSet)
	cmd.Flags().StringVar(&status, "status", "", "filter by status (e.g. draft, scheduled, published, error)")
	cmd.Flags().StringVar(&tag, "tag", "", "filter by tag id")
	cmd.Flags().StringVar(&orderBy, "order-by", "", "sort order (provider-defined)")
	registerPaging(cmd, &limit, &offset)
	return cmd
}

func (s *Service) newDraftGetCmd(token string) *cobra.Command {
	var socialSet, id string
	var excludeMarkers bool
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Read one draft's full content + status/publish_state (GET /v2/social-sets/{id}/drafts/{draft_id})",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if excludeMarkers {
				q.Set("exclude_comment_markers", "true")
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, scopedPath(socialSet, "/drafts/"+id), q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	addSocialSetFlag(cmd, &socialSet)
	cmd.Flags().StringVar(&id, "id", "", "draft id; required")
	_ = cmd.MarkFlagRequired("id")
	cmd.Flags().BoolVar(&excludeMarkers, "exclude-comment-markers", false, "strip inline comment markers from returned content")
	return cmd
}

func (s *Service) newDraftCreateCmd(token string) *cobra.Command {
	var socialSet, data, publishAt string
	var texts, platforms, mediaIDs []string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create + optionally schedule/publish a draft (POST /v2/social-sets/{id}/drafts)",
		Annotations: writeAction,
		Long: "Create a draft. Provide EITHER --data '<raw json>' (full platforms body, " +
			"the honest path for platform-specific/nested content) OR the thin " +
			"convenience flags (--text repeatable builds a thread, --platform " +
			"repeatable defaults to x, --publish-at now|next-free-slot|<iso8601>, " +
			"--media-id attaches media to the first post). --data and the typed " +
			"flags are mutually exclusive.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			typed := len(texts) > 0 || len(platforms) > 0 || len(mediaIDs) > 0 || publishAt != ""
			if data != "" && typed {
				return &usageError{msg: "--data and the typed flags (--text/--platform/--publish-at/--media-id) are mutually exclusive"}
			}
			var payload any
			if data != "" {
				decoded, err := decodeJSONFlag("data", data)
				if err != nil {
					return err
				}
				payload = decoded
			} else {
				if len(texts) == 0 {
					return &usageError{msg: "provide --text (repeatable, one per thread post) or --data '<raw json>'"}
				}
				payload = buildDraftBody(texts, platforms, mediaIDs, publishAt)
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, scopedPath(socialSet, "/drafts"), nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	addSocialSetFlag(cmd, &socialSet)
	cmd.Flags().StringVar(&data, "data", "", "raw JSON request body (mutually exclusive with the typed flags)")
	cmd.Flags().StringArrayVar(&texts, "text", nil, "post text; repeat for a thread (one post per --text)")
	cmd.Flags().StringArrayVar(&platforms, "platform", nil, "target platform (repeatable; default x). e.g. x, linkedin, threads, bluesky, mastodon")
	cmd.Flags().StringArrayVar(&mediaIDs, "media-id", nil, "media id to attach to the first post (repeatable)")
	cmd.Flags().StringVar(&publishAt, "publish-at", "", "now | next-free-slot | ISO-8601 datetime with timezone (omit to save a plain draft)")
	return cmd
}

// buildDraftBody assembles the verified v2 create-draft body from the thin
// convenience flags: platforms is an object keyed by platform name, each with
// enabled=true and a posts array; a thread is multiple posts. media_ids attach
// to the first post. publish_at is top-level when set.
func buildDraftBody(texts, platforms, mediaIDs []string, publishAt string) map[string]any {
	posts := make([]map[string]any, 0, len(texts))
	for i, text := range texts {
		post := map[string]any{"text": text}
		if i == 0 && len(mediaIDs) > 0 {
			post["media_ids"] = mediaIDs
		}
		posts = append(posts, post)
	}
	if len(platforms) == 0 {
		platforms = []string{"x"}
	}
	platformBody := make(map[string]any, len(platforms))
	for _, p := range platforms {
		platformBody[p] = map[string]any{"enabled": true, "posts": posts}
	}
	body := map[string]any{"platforms": platformBody}
	if publishAt != "" {
		body["publish_at"] = publishAt
	}
	return body
}

func (s *Service) newDraftUpdateCmd(token string) *cobra.Command {
	var socialSet, id, data string
	cmd := &cobra.Command{
		Use:         "update",
		Short:       "Edit / reschedule / publish an existing draft (PATCH /v2/social-sets/{id}/drafts/{draft_id})",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			decoded, err := decodeJSONFlag("data", data)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPatch, scopedPath(socialSet, "/drafts/"+id), nil, decoded)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	addSocialSetFlag(cmd, &socialSet)
	cmd.Flags().StringVar(&id, "id", "", "draft id; required")
	_ = cmd.MarkFlagRequired("id")
	cmd.Flags().StringVar(&data, "data", "", "raw JSON patch body; required")
	_ = cmd.MarkFlagRequired("data")
	return cmd
}

func (s *Service) newDraftDeleteCmd(token string) *cobra.Command {
	var socialSet, id string
	cmd := &cobra.Command{
		Use:         "delete",
		Short:       "Delete a draft (DELETE /v2/social-sets/{id}/drafts/{draft_id})",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := s.call(cmd.Context(), token, http.MethodDelete, scopedPath(socialSet, "/drafts/"+id), nil, nil); err != nil {
				return err
			}
			return s.emitValue(map[string]any{"deleted": true, "id": id})
		},
	}
	addSocialSetFlag(cmd, &socialSet)
	cmd.Flags().StringVar(&id, "id", "", "draft id; required")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}
