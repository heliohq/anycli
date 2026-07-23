package bluesky

import (
	"encoding/json"
	"fmt"
	"io"
)

func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

func (s *Service) emitValue(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("bluesky: encode output: %w", err)
	}
	return s.emit(body)
}

// authorView is the thin, agent-facing author shape.
type authorView struct {
	DID         string `json:"did"`
	Handle      string `json:"handle"`
	DisplayName string `json:"display_name,omitempty"`
}

// postView is the provider-neutral post shape emitted for reads. at:// URIs and
// cids are opaque handles the AI passes back verbatim for delete/like/reply.
type postView struct {
	URI         string     `json:"uri"`
	CID         string     `json:"cid"`
	Author      authorView `json:"author"`
	Text        string     `json:"text"`
	CreatedAt   string     `json:"created_at,omitempty"`
	ReplyCount  int        `json:"reply_count"`
	RepostCount int        `json:"repost_count"`
	LikeCount   int        `json:"like_count"`
}

// profileView is the provider-neutral profile shape.
type profileView struct {
	DID            string `json:"did"`
	Handle         string `json:"handle"`
	DisplayName    string `json:"display_name,omitempty"`
	Description    string `json:"description,omitempty"`
	FollowersCount int    `json:"followers_count"`
	FollowsCount   int    `json:"follows_count"`
	PostsCount     int    `json:"posts_count"`
}

// rawPost mirrors the XRPC app.bsky.feed.defs#postView fields we surface.
type rawPost struct {
	URI    string `json:"uri"`
	CID    string `json:"cid"`
	Author struct {
		DID         string `json:"did"`
		Handle      string `json:"handle"`
		DisplayName string `json:"displayName"`
	} `json:"author"`
	Record struct {
		Text      string `json:"text"`
		CreatedAt string `json:"createdAt"`
	} `json:"record"`
	ReplyCount  int `json:"replyCount"`
	RepostCount int `json:"repostCount"`
	LikeCount   int `json:"likeCount"`
}

func (p rawPost) shape() postView {
	return postView{
		URI: p.URI,
		CID: p.CID,
		Author: authorView{
			DID:         p.Author.DID,
			Handle:      p.Author.Handle,
			DisplayName: p.Author.DisplayName,
		},
		Text:        p.Record.Text,
		CreatedAt:   p.Record.CreatedAt,
		ReplyCount:  p.ReplyCount,
		RepostCount: p.RepostCount,
		LikeCount:   p.LikeCount,
	}
}

type postListView struct {
	Posts  []postView `json:"posts"`
	Cursor string     `json:"cursor,omitempty"`
}

func shapePostList(posts []rawPost, cursor string) postListView {
	out := postListView{Posts: make([]postView, 0, len(posts)), Cursor: cursor}
	for _, p := range posts {
		out.Posts = append(out.Posts, p.shape())
	}
	return out
}
