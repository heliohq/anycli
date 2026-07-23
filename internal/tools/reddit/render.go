package reddit

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// thing is Reddit's generic {kind, data} envelope. kind is a type prefix:
// t1=comment, t2=account, t3=link/post, t4=message, t5=subreddit, more=elided
// children in a comment tree.
type thing struct {
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data"`
}

// listing is Reddit's paginated {kind:"Listing", data:{after, children}}.
type listing struct {
	Kind string `json:"kind"`
	Data struct {
		After    *string `json:"after"`
		Children []thing `json:"children"`
	} `json:"data"`
}

// thingData is the superset of the entity fields the tool surfaces; unused
// fields for a given kind stay zero. replies is a nested comment listing (or the
// empty string Reddit sends for a leaf comment), decoded lazily.
type thingData struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Title        string          `json:"title"`
	Author       string          `json:"author"`
	Subreddit    string          `json:"subreddit"`
	Score        int             `json:"score"`
	NumComments  int             `json:"num_comments"`
	CreatedUTC   float64         `json:"created_utc"`
	Permalink    string          `json:"permalink"`
	URL          string          `json:"url"`
	SelfText     string          `json:"selftext"`
	Body         string          `json:"body"`
	ParentID     string          `json:"parent_id"`
	Subject      string          `json:"subject"`
	New          bool            `json:"new"`
	WasComment   bool            `json:"was_comment"`
	Dest         string          `json:"dest"`
	DisplayName  string          `json:"display_name"`
	PublicDesc   string          `json:"public_description"`
	Subscribers  int             `json:"subscribers"`
	LinkKarma    int             `json:"link_karma"`
	CommentKarma int             `json:"comment_karma"`
	Count        int             `json:"count"`
	Children     []string        `json:"children"`
	Replies      json.RawMessage `json:"replies"`
}

// postItem is the flat, provider-neutral post shape emitted under --json.
type postItem struct {
	ID          string  `json:"id"`
	Fullname    string  `json:"fullname"`
	Title       string  `json:"title"`
	Author      string  `json:"author"`
	Subreddit   string  `json:"subreddit"`
	Score       int     `json:"score"`
	NumComments int     `json:"num_comments"`
	CreatedUTC  float64 `json:"created_utc"`
	Permalink   string  `json:"permalink"`
	URL         string  `json:"url,omitempty"`
	SelfText    string  `json:"selftext,omitempty"`
}

// commentItem is the flat, provider-neutral comment shape emitted under --json.
type commentItem struct {
	ID         string  `json:"id"`
	Fullname   string  `json:"fullname"`
	Author     string  `json:"author"`
	Body       string  `json:"body"`
	Score      int     `json:"score"`
	ParentID   string  `json:"parent_id"`
	Depth      int     `json:"depth"`
	CreatedUTC float64 `json:"created_utc"`
}

// moreItem surfaces an unexpanded "more comments" stub rather than dropping it.
type moreItem struct {
	Kind     string `json:"kind"`
	Count    int    `json:"count"`
	ParentID string `json:"parent_id"`
}

func toPostItem(d thingData) postItem {
	return postItem{
		ID: d.ID, Fullname: d.Name, Title: d.Title, Author: d.Author,
		Subreddit: d.Subreddit, Score: d.Score, NumComments: d.NumComments,
		CreatedUTC: d.CreatedUTC, Permalink: d.Permalink, URL: d.URL, SelfText: d.SelfText,
	}
}

func toCommentItem(d thingData, depth int) commentItem {
	return commentItem{
		ID: d.ID, Fullname: d.Name, Author: d.Author, Body: d.Body,
		Score: d.Score, ParentID: d.ParentID, Depth: depth, CreatedUTC: d.CreatedUTC,
	}
}

// emitValue writes one value as a single JSON line to stdout.
func (s *Service) emitValue(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("reddit: encode output: %v", err), err: err}
	}
	_, err = io.WriteString(s.stdout(), string(b)+"\n")
	return err
}

// emitLine writes one plain-text line to stdout.
func (s *Service) emitLine(line string) error {
	_, err := io.WriteString(s.stdout(), line+"\n")
	return err
}

// decodeThingData unmarshals a thing's data into thingData.
func decodeThingData(raw json.RawMessage) (thingData, error) {
	var d thingData
	if err := json.Unmarshal(raw, &d); err != nil {
		return thingData{}, &apiError{msg: fmt.Sprintf("reddit: decode entity: %v", err), err: err}
	}
	return d, nil
}

// emitPostListing parses a Listing of posts, emitting each as a flat item
// (JSONL under --json, one terse line otherwise) followed by an {"after":…}
// cursor object when more pages exist.
func (s *Service) emitPostListing(cmd cmdJSON, body []byte) error {
	var l listing
	if err := json.Unmarshal(body, &l); err != nil {
		return &apiError{msg: fmt.Sprintf("reddit: decode listing: %v", err), err: err}
	}
	for _, c := range l.Data.Children {
		d, err := decodeThingData(c.Data)
		if err != nil {
			return err
		}
		if cmd.json() {
			if err := s.emitValue(toPostItem(d)); err != nil {
				return err
			}
			continue
		}
		if err := s.emitLine(tersePost(d)); err != nil {
			return err
		}
	}
	return s.emitCursor(cmd, l.Data.After)
}

// emitSubredditListing parses a Listing of subreddits (subscriptions).
func (s *Service) emitSubredditListing(cmd cmdJSON, body []byte) error {
	var l listing
	if err := json.Unmarshal(body, &l); err != nil {
		return &apiError{msg: fmt.Sprintf("reddit: decode listing: %v", err), err: err}
	}
	for _, c := range l.Data.Children {
		d, err := decodeThingData(c.Data)
		if err != nil {
			return err
		}
		if cmd.json() {
			if err := s.emitValue(toSubreddit(d)); err != nil {
				return err
			}
			continue
		}
		if err := s.emitLine(fmt.Sprintf("r/%s\t%d subscribers", d.DisplayName, d.Subscribers)); err != nil {
			return err
		}
	}
	return s.emitCursor(cmd, l.Data.After)
}

// emitCommentListing parses a Listing of comments (user comments, inbox).
func (s *Service) emitCommentListing(cmd cmdJSON, body []byte) error {
	var l listing
	if err := json.Unmarshal(body, &l); err != nil {
		return &apiError{msg: fmt.Sprintf("reddit: decode listing: %v", err), err: err}
	}
	for _, c := range l.Data.Children {
		d, err := decodeThingData(c.Data)
		if err != nil {
			return err
		}
		if cmd.json() {
			if err := s.emitValue(toCommentItem(d, 0)); err != nil {
				return err
			}
			continue
		}
		if err := s.emitLine(terseComment(d, 0)); err != nil {
			return err
		}
	}
	return s.emitCursor(cmd, l.Data.After)
}

func (s *Service) emitCursor(cmd cmdJSON, after *string) error {
	if after == nil || *after == "" {
		return nil
	}
	if cmd.json() {
		return s.emitValue(map[string]string{"after": *after})
	}
	return s.emitLine("after: " + *after)
}

// emitCommentTree flattens the comment listing returned by GET /comments/{id}.
// The response is a two-element array: [0] the post listing, [1] the comment
// forest. Comments nest via data.replies; "more" kinds are surfaced as stubs.
func (s *Service) emitCommentTree(cmd cmdJSON, body []byte) error {
	var pair []listing
	if err := json.Unmarshal(body, &pair); err != nil {
		return &apiError{msg: fmt.Sprintf("reddit: decode comments: %v", err), err: err}
	}
	if len(pair) < 2 {
		return nil
	}
	return s.walkComments(cmd, pair[1].Data.Children, 0)
}

func (s *Service) walkComments(cmd cmdJSON, children []thing, depth int) error {
	for _, c := range children {
		if c.Kind == "more" {
			d, err := decodeThingData(c.Data)
			if err != nil {
				return err
			}
			stub := moreItem{Kind: "more", Count: d.Count, ParentID: d.ParentID}
			if cmd.json() {
				if err := s.emitValue(stub); err != nil {
					return err
				}
			} else if err := s.emitLine(fmt.Sprintf("%s… %d more replies", strings.Repeat("  ", depth), d.Count)); err != nil {
				return err
			}
			continue
		}
		d, err := decodeThingData(c.Data)
		if err != nil {
			return err
		}
		if cmd.json() {
			if err := s.emitValue(toCommentItem(d, depth)); err != nil {
				return err
			}
		} else if err := s.emitLine(terseComment(d, depth)); err != nil {
			return err
		}
		// Recurse into nested replies (Reddit sends "" for a leaf comment).
		if len(d.Replies) > 0 && string(d.Replies) != `""` && string(d.Replies) != "null" {
			var nested listing
			if err := json.Unmarshal(d.Replies, &nested); err == nil {
				if err := s.walkComments(cmd, nested.Data.Children, depth+1); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func toSubreddit(d thingData) map[string]any {
	return map[string]any{
		"id":                 d.ID,
		"fullname":           d.Name,
		"display_name":       d.DisplayName,
		"title":              d.Title,
		"public_description": d.PublicDesc,
		"subscribers":        d.Subscribers,
		"url":                d.URL,
	}
}

func tersePost(d thingData) string {
	return fmt.Sprintf("%s\tr/%s\tu/%s\t%d pts\t%d cmts\t%s",
		d.Name, d.Subreddit, d.Author, d.Score, d.NumComments, d.Title)
}

func terseComment(d thingData, depth int) string {
	body := strings.ReplaceAll(strings.TrimSpace(d.Body), "\n", " ")
	if len(body) > 140 {
		body = body[:140] + "…"
	}
	return fmt.Sprintf("%su/%s\t%d pts\t%s", strings.Repeat("  ", depth), d.Author, d.Score, body)
}

// cmdJSON abstracts "is --json set" so render helpers stay decoupled from cobra.
type cmdJSON interface{ json() bool }

type jsonFlag bool

func (j jsonFlag) json() bool { return bool(j) }

// intToStr is a tiny helper for building form/query values.
func intToStr(n int) string { return strconv.Itoa(n) }
