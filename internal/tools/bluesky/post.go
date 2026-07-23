package bluesky

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func (s *Service) newPostCmd(sess *session) *cobra.Command {
	cmd := &cobra.Command{Use: "post", Short: "Posts"}
	cmd.AddCommand(
		s.newPostCreateCmd(sess),
		s.newPostDeleteCmd(sess),
		s.newPostGetCmd(sess),
	)
	return cmd
}

func (s *Service) newPostCreateCmd(sess *session) *cobra.Command {
	var text, replyTo, quote, lang string
	var images, alts []string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a post (text, reply, quote, links, images)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(text) == "" {
				return fmt.Errorf("--text must not be empty")
			}
			if len(images) > 4 {
				return fmt.Errorf("at most 4 --image values are supported per post")
			}
			ctx := cmd.Context()

			record := map[string]any{
				"$type":     collectionPost,
				"text":      text,
				"createdAt": nowRFC3339(),
			}
			if lang != "" {
				record["langs"] = []string{lang}
			}
			if facets := detectFacets(text, sess.mentionResolver(ctx)); len(facets) > 0 {
				record["facets"] = facets
			}
			if replyTo != "" {
				reply, err := sess.buildReply(ctx, replyTo)
				if err != nil {
					return err
				}
				record["reply"] = reply
			}
			if err := s.attachEmbed(ctx, sess, record, images, alts, quote); err != nil {
				return err
			}

			resp, err := sess.createRecord(ctx, collectionPost, record)
			if err != nil {
				return err
			}
			return s.emitValue(map[string]string{
				"uri":    resp.URI,
				"cid":    resp.CID,
				"handle": sess.handle,
			})
		},
	}
	cmd.Flags().StringVar(&text, "text", "", "post text (links and #hashtags become facets automatically)")
	cmd.Flags().StringVar(&replyTo, "reply-to", "", "at:// URI of the post to reply to")
	cmd.Flags().StringVar(&quote, "quote", "", "at:// URI of the post to quote")
	cmd.Flags().StringArrayVar(&images, "image", nil, "path to an image to attach (repeatable, maximum 4)")
	cmd.Flags().StringArrayVar(&alts, "alt", nil, "alt text for the image at the same position (repeatable)")
	cmd.Flags().StringVar(&lang, "lang", "", "BCP-47 language tag for the post text, e.g. en")
	_ = cmd.MarkFlagRequired("text")
	return cmd
}

// attachEmbed wires image and/or quote embeds onto the record. Images alone use
// app.bsky.embed.images; a quote alone uses app.bsky.embed.record; both
// together use app.bsky.embed.recordWithMedia.
func (s *Service) attachEmbed(ctx context.Context, sess *session, record map[string]any, images, alts []string, quote string) error {
	var media map[string]any
	if len(images) > 0 {
		imgs, err := sess.uploadImages(ctx, images, alts)
		if err != nil {
			return err
		}
		media = map[string]any{"$type": "app.bsky.embed.images", "images": imgs}
	}

	var recordEmbed map[string]any
	if quote != "" {
		ref, _, err := sess.fetchPostRef(ctx, quote)
		if err != nil {
			return err
		}
		recordEmbed = map[string]any{
			"$type":  "app.bsky.embed.record",
			"record": recordRef{URI: ref.URI, CID: ref.CID},
		}
	}

	switch {
	case media != nil && recordEmbed != nil:
		record["embed"] = map[string]any{
			"$type":  "app.bsky.embed.recordWithMedia",
			"record": recordEmbed,
			"media":  media,
		}
	case media != nil:
		record["embed"] = media
	case recordEmbed != nil:
		record["embed"] = recordEmbed
	}
	return nil
}

// uploadImages reads each path, uploads the bytes as a blob, and returns the
// app.bsky.embed.images image entries with alt text (empty alt warns on stderr).
func (se *session) uploadImages(ctx context.Context, paths, alts []string) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(paths))
	for i, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("bluesky: read image %q: %w", path, err)
		}
		blob, err := se.uploadBlob(ctx, data, detectImageContentType(data))
		if err != nil {
			return nil, err
		}
		alt := ""
		if i < len(alts) {
			alt = alts[i]
		}
		if alt == "" {
			fmt.Fprintf(se.svc.stderr(), "warning: image %q has no --alt text (accessibility)\n", path)
		}
		out = append(out, map[string]any{"alt": alt, "image": json.RawMessage(blob)})
	}
	return out, nil
}

// buildReply resolves the reply parent and thread root refs from the target
// post's at:// URI (the root is the parent's own root, or the parent itself).
func (se *session) buildReply(ctx context.Context, replyTo string) (map[string]any, error) {
	ref, root, err := se.fetchPostRef(ctx, replyTo)
	if err != nil {
		return nil, err
	}
	parent := recordRef{URI: ref.URI, CID: ref.CID}
	rootRef := parent
	if root != nil {
		rootRef = *root
	}
	return map[string]any{"root": rootRef, "parent": parent}, nil
}

// fetchPostRef looks up a post by at:// URI and returns its {uri, cid} plus the
// thread root ref carried in its reply, if any.
func (se *session) fetchPostRef(ctx context.Context, uri string) (recordRef, *recordRef, error) {
	if _, err := parseATURI(uri); err != nil {
		return recordRef{}, nil, err
	}
	query := url.Values{"uris": {uri}}
	body, err := se.call(ctx, http.MethodGet, "app.bsky.feed.getPosts", query, nil)
	if err != nil {
		return recordRef{}, nil, err
	}
	var resp struct {
		Posts []struct {
			URI    string `json:"uri"`
			CID    string `json:"cid"`
			Record struct {
				Reply *struct {
					Root recordRef `json:"root"`
				} `json:"reply"`
			} `json:"record"`
		} `json:"posts"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return recordRef{}, nil, fmt.Errorf("bluesky: decode getPosts response: %w", err)
	}
	if len(resp.Posts) == 0 {
		return recordRef{}, nil, fmt.Errorf("bluesky: no post found at %q", uri)
	}
	p := resp.Posts[0]
	ref := recordRef{URI: p.URI, CID: p.CID}
	if p.Record.Reply != nil {
		root := p.Record.Reply.Root
		return ref, &root, nil
	}
	return ref, nil, nil
}

func (s *Service) newPostDeleteCmd(sess *session) *cobra.Command {
	var uri string
	cmd := &cobra.Command{
		Use:         "delete",
		Short:       "Delete a post by its at:// URI",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			parsed, err := parseATURI(uri)
			if err != nil {
				return err
			}
			if err := sess.deleteRecord(cmd.Context(), parsed); err != nil {
				return err
			}
			return s.emitValue(map[string]string{"uri": uri, "deleted": "true"})
		},
	}
	cmd.Flags().StringVar(&uri, "uri", "", "at:// URI of the post to delete")
	_ = cmd.MarkFlagRequired("uri")
	return cmd
}

func (s *Service) newPostGetCmd(sess *session) *cobra.Command {
	var uri string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get a post (and its thread root) by at:// URI",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := parseATURI(uri); err != nil {
				return err
			}
			query := url.Values{"uri": {uri}}
			body, err := sess.call(cmd.Context(), http.MethodGet, "app.bsky.feed.getPostThread", query, nil)
			if err != nil {
				return err
			}
			var resp struct {
				Thread struct {
					Post rawPost `json:"post"`
				} `json:"thread"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("bluesky: decode thread: %w", err)
			}
			return s.emitValue(resp.Thread.Post.shape())
		},
	}
	cmd.Flags().StringVar(&uri, "uri", "", "at:// URI of the post to fetch")
	_ = cmd.MarkFlagRequired("uri")
	return cmd
}

// mentionResolver returns a best-effort handle→DID resolver bound to the session
// context, used only for facet computation. Failures are swallowed so a post
// never fails because a mention could not resolve.
func (se *session) mentionResolver(ctx context.Context) func(string) (string, bool) {
	return func(handle string) (string, bool) {
		query := url.Values{"handle": {handle}}
		body, err := se.call(ctx, http.MethodGet, "com.atproto.identity.resolveHandle", query, nil)
		if err != nil {
			return "", false
		}
		var resp struct {
			DID string `json:"did"`
		}
		if err := json.Unmarshal(body, &resp); err != nil || resp.DID == "" {
			return "", false
		}
		return resp.DID, true
	}
}
