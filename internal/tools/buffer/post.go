package buffer

import (
	"github.com/spf13/cobra"
)

// Scheduling enum values (verified against the official examples). Buffer keeps
// schedulingType as `automatic` for both queued and custom-scheduled posts;
// `mode` differentiates queue insertion from a specific dueAt.
const (
	schedulingTypeAutomatic = "automatic"
	modeAddToQueue          = "addToQueue"
	modeCustomScheduled     = "customScheduled"
)

const postsQuery = `query($input: PostsInput!, $first: Int, $after: String) {
  posts(input: $input, first: $first, after: $after) {
    pageInfo { startCursor endCursor hasNextPage }
    edges { node { id text createdAt channelId } }
  }
}`

const createPostMutation = `mutation($input: CreatePostInput!) {
  createPost(input: $input) {
    __typename
    ... on PostActionSuccess { post { id text dueAt } }
    ... on MutationError { message }
  }
}`

const editPostMutation = `mutation($input: EditPostInput!) {
  editPost(input: $input) {
    __typename
    ... on PostActionSuccess { post { id text dueAt } }
    ... on MutationError { message }
  }
}`

const deletePostMutation = `mutation($input: DeletePostInput!) {
  deletePost(input: $input) {
    __typename
    ... on DeletePostSuccess { id }
    ... on MutationError { message }
  }
}`

// postNode is the provider-neutral projection of one post read from the posts
// connection.
type postNode struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	CreatedAt string `json:"createdAt"`
	ChannelID string `json:"channelId"`
}

func (s *Service) newPostListCmd(token string) *cobra.Command {
	var org, channelID, status, after string
	var first int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List posts in an organization (optionally filtered by channel/status)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if org == "" {
				return &usageError{msg: "--org is required"}
			}
			input := map[string]any{"organizationId": org}
			filter := map[string]any{}
			if channelID != "" {
				filter["channelIds"] = []string{channelID}
			}
			if status != "" {
				// PostsFiltersInput.status is [PostStatus!]; the typed variable
				// lets the server coerce the string to the enum member.
				filter["status"] = []string{status}
			}
			if len(filter) > 0 {
				input["filter"] = filter
			}
			variables := map[string]any{"input": input}
			if first > 0 {
				variables["first"] = first
			}
			if after != "" {
				variables["after"] = after
			}

			data, err := s.gql(cmd.Context(), token, postsQuery, variables)
			if err != nil {
				return err
			}
			var result struct {
				PostsResults struct {
					PageInfo map[string]any `json:"pageInfo"`
					Edges    []struct {
						Node postNode `json:"node"`
					} `json:"edges"`
				}
			}
			if err := decodeField(data, "posts", &result.PostsResults); err != nil {
				return err
			}
			posts := make([]postNode, 0, len(result.PostsResults.Edges))
			for _, e := range result.PostsResults.Edges {
				posts = append(posts, e.Node)
			}
			return s.emitValue(map[string]any{
				"posts":    posts,
				"pageInfo": result.PostsResults.PageInfo,
			})
		},
	}
	cmd.Flags().StringVar(&org, "org", "", "organization (workspace) id (required)")
	cmd.Flags().StringVar(&channelID, "channel", "", "filter to one channel id (optional)")
	cmd.Flags().StringVar(&status, "status", "", "filter by post status, e.g. sent|draft (optional)")
	cmd.Flags().IntVar(&first, "first", 0, "max posts to return (optional)")
	cmd.Flags().StringVar(&after, "after", "", "pagination cursor from a prior pageInfo.endCursor (optional)")
	return cmd
}

func (s *Service) newPostCreateCmd(token string) *cobra.Command {
	var channelID, text, mode, dueAt, assetsJSON, metadataJSON string
	var draft bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a post on a channel (queue or custom-scheduled)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if channelID == "" {
				return &usageError{msg: "--channel is required"}
			}
			if text == "" {
				return &usageError{msg: "--text is required"}
			}
			if mode == "" {
				mode = modeAddToQueue
			}
			if mode != modeAddToQueue && mode != modeCustomScheduled {
				return &usageError{msg: "--mode must be addToQueue or customScheduled"}
			}
			if mode == modeCustomScheduled && dueAt == "" {
				return &usageError{msg: "--due-at is required when --mode customScheduled"}
			}

			input := map[string]any{
				"channelId":      channelID,
				"text":           text,
				"schedulingType": schedulingTypeAutomatic,
				"mode":           mode,
			}
			if dueAt != "" {
				input["dueAt"] = dueAt
			}
			if draft {
				input["saveToDraft"] = true
			}
			if err := applyAssetsMetadata(input, assetsJSON, metadataJSON); err != nil {
				return err
			}

			data, err := s.gql(cmd.Context(), token, createPostMutation, map[string]any{"input": input})
			if err != nil {
				return err
			}
			payload, err := mutationSuccess(data, "createPost", "PostActionSuccess")
			if err != nil {
				return err
			}
			return s.emitValue(postFromPayload(payload, channelID))
		},
	}
	cmd.Flags().StringVar(&channelID, "channel", "", "target channel id (required)")
	cmd.Flags().StringVar(&text, "text", "", "post text (required)")
	cmd.Flags().StringVar(&mode, "mode", "", "addToQueue (default) or customScheduled")
	cmd.Flags().StringVar(&dueAt, "due-at", "", "ISO-8601 UTC time (required for customScheduled)")
	cmd.Flags().BoolVar(&draft, "draft", false, "save as a draft instead of publishing/queuing")
	cmd.Flags().StringVar(&assetsJSON, "assets-json", "", "raw Buffer assets object as JSON (optional)")
	cmd.Flags().StringVar(&metadataJSON, "metadata-json", "", "raw per-service metadata object as JSON (optional)")
	return cmd
}

func (s *Service) newPostEditCmd(token string) *cobra.Command {
	var id, text, mode, dueAt, assetsJSON, metadataJSON string
	cmd := &cobra.Command{
		Use:   "edit",
		Short: "Edit an existing post",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if id == "" {
				return &usageError{msg: "--id is required"}
			}
			if mode == "" {
				mode = modeAddToQueue
			}
			if mode != modeAddToQueue && mode != modeCustomScheduled {
				return &usageError{msg: "--mode must be addToQueue or customScheduled"}
			}
			if mode == modeCustomScheduled && dueAt == "" {
				return &usageError{msg: "--due-at is required when --mode customScheduled"}
			}

			// EditPostInput requires id, schedulingType, and mode.
			input := map[string]any{
				"id":             id,
				"schedulingType": schedulingTypeAutomatic,
				"mode":           mode,
			}
			if text != "" {
				input["text"] = text
			}
			if dueAt != "" {
				input["dueAt"] = dueAt
			}
			if err := applyAssetsMetadata(input, assetsJSON, metadataJSON); err != nil {
				return err
			}

			data, err := s.gql(cmd.Context(), token, editPostMutation, map[string]any{"input": input})
			if err != nil {
				return err
			}
			payload, err := mutationSuccess(data, "editPost", "PostActionSuccess")
			if err != nil {
				return err
			}
			return s.emitValue(postFromPayload(payload, ""))
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "post id (required)")
	cmd.Flags().StringVar(&text, "text", "", "new post text (optional)")
	cmd.Flags().StringVar(&mode, "mode", "", "addToQueue (default) or customScheduled")
	cmd.Flags().StringVar(&dueAt, "due-at", "", "ISO-8601 UTC time (required for customScheduled)")
	cmd.Flags().StringVar(&assetsJSON, "assets-json", "", "raw Buffer assets object as JSON (optional)")
	cmd.Flags().StringVar(&metadataJSON, "metadata-json", "", "raw per-service metadata object as JSON (optional)")
	return cmd
}

func (s *Service) newPostDeleteCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a post",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if id == "" {
				return &usageError{msg: "--id is required"}
			}
			data, err := s.gql(cmd.Context(), token, deletePostMutation, map[string]any{
				"input": map[string]any{"id": id},
			})
			if err != nil {
				return err
			}
			payload, err := mutationSuccess(data, "deletePost", "DeletePostSuccess")
			if err != nil {
				return err
			}
			deletedID, _ := payload["id"].(string)
			return s.emitValue(map[string]any{"id": deletedID})
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "post id (required)")
	return cmd
}

// postFromPayload projects a PostActionSuccess payload's `post` object into the
// neutral shape. channelID is filled in from the request when the response
// omits it (create knows the channel; the response `post` selection does not
// re-emit it).
func postFromPayload(payload map[string]any, channelID string) map[string]any {
	out := map[string]any{}
	if post, ok := payload["post"].(map[string]any); ok {
		out["id"], _ = post["id"].(string)
		out["text"], _ = post["text"].(string)
		out["dueAt"] = post["dueAt"]
	}
	if channelID != "" {
		out["channelId"] = channelID
	}
	return out
}

// applyAssetsMetadata validates and attaches the optional raw --assets-json /
// --metadata-json inputs onto a post input, enforcing Buffer's documented
// mutual exclusion: a post carrying uploaded videos in assets and a
// link-attachment in per-service metadata is rejected (Buffer ignores the
// video). Surfacing it as a usage error (exit 2) beats silently dropping data.
func applyAssetsMetadata(input map[string]any, assetsJSON, metadataJSON string) error {
	var assets, metadata any
	if assetsJSON != "" {
		v, err := decodeJSONFlag("assets-json", assetsJSON)
		if err != nil {
			return err
		}
		assets = v
	}
	if metadataJSON != "" {
		v, err := decodeJSONFlag("metadata-json", metadataJSON)
		if err != nil {
			return err
		}
		metadata = v
	}
	if hasVideos(assets) && hasLinkAttachment(metadata) {
		return &usageError{msg: "assets.videos and metadata.<service>.linkAttachment are mutually exclusive; supply only one"}
	}
	if assets != nil {
		input["assets"] = assets
	}
	if metadata != nil {
		input["metadata"] = metadata
	}
	return nil
}

// hasVideos reports whether a decoded assets object carries a non-empty
// `videos` array.
func hasVideos(assets any) bool {
	obj, ok := assets.(map[string]any)
	if !ok {
		return false
	}
	videos, ok := obj["videos"].([]any)
	return ok && len(videos) > 0
}

// hasLinkAttachment reports whether any per-service metadata entry carries a
// `linkAttachment`. metadata is keyed by service (e.g. {"twitter": {...}}).
func hasLinkAttachment(metadata any) bool {
	obj, ok := metadata.(map[string]any)
	if !ok {
		return false
	}
	for _, v := range obj {
		if service, ok := v.(map[string]any); ok {
			if _, present := service["linkAttachment"]; present {
				return true
			}
		}
	}
	return false
}
