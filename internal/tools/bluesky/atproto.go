package bluesky

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Lexicon collection ids and record $types.
const (
	collectionPost   = "app.bsky.feed.post"
	collectionLike   = "app.bsky.feed.like"
	collectionRepost = "app.bsky.feed.repost"
	collectionFollow = "app.bsky.graph.follow"
)

// nowRFC3339 is overridable in tests for deterministic createdAt assertions.
var nowRFC3339 = func() string { return time.Now().UTC().Format(time.RFC3339) }

// detectImageContentType sniffs the image bytes and returns the MIME type
// uploadBlob should record. Unknown content falls back to image/jpeg so the
// blob still uploads with an image MIME.
func detectImageContentType(data []byte) string {
	ct := http.DetectContentType(data)
	if !strings.HasPrefix(ct, "image/") {
		return "image/jpeg"
	}
	return ct
}

// atURI is a parsed at:// URI: at://<authority>/<collection>/<rkey>.
type atURI struct {
	Authority  string
	Collection string
	RKey       string
}

func parseATURI(raw string) (atURI, error) {
	rest, ok := strings.CutPrefix(raw, "at://")
	if !ok {
		return atURI{}, fmt.Errorf("uri must be an at:// URI, got %q", raw)
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return atURI{}, fmt.Errorf("at:// URI must be at://<did>/<collection>/<rkey>, got %q", raw)
	}
	return atURI{Authority: parts[0], Collection: parts[1], RKey: parts[2]}, nil
}

// recordRef is the {uri, cid} subject echoed on create and required by
// like/repost subjects.
type recordRef struct {
	URI string `json:"uri"`
	CID string `json:"cid"`
}

// createRecordResponse is the com.atproto.repo.createRecord result.
type createRecordResponse struct {
	URI string `json:"uri"`
	CID string `json:"cid"`
}

// createRecord writes a record into the connected user's repo and returns its
// uri + cid.
func (se *session) createRecord(ctx context.Context, collection string, record any) (createRecordResponse, error) {
	if err := se.ensure(ctx); err != nil {
		return createRecordResponse{}, err
	}
	payload := map[string]any{
		"repo":       se.did,
		"collection": collection,
		"record":     record,
	}
	body, err := se.call(ctx, http.MethodPost, "com.atproto.repo.createRecord", nil, payload)
	if err != nil {
		return createRecordResponse{}, err
	}
	var resp createRecordResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return createRecordResponse{}, fmt.Errorf("bluesky: decode createRecord response: %w", err)
	}
	return resp, nil
}

// deleteRecord removes a record identified by an at:// URI from the connected
// user's repo (the authority must be the connected DID).
func (se *session) deleteRecord(ctx context.Context, uri atURI) error {
	payload := map[string]string{
		"repo":       uri.Authority,
		"collection": uri.Collection,
		"rkey":       uri.RKey,
	}
	_, err := se.call(ctx, http.MethodPost, "com.atproto.repo.deleteRecord", nil, payload)
	return err
}

// resolveActorDID resolves a handle-or-DID actor to its DID via getProfile,
// which also validates that the actor exists.
func (se *session) resolveActorDID(ctx context.Context, actor string) (string, error) {
	prof, err := se.getProfile(ctx, actor)
	if err != nil {
		return "", err
	}
	if prof.DID == "" {
		return "", fmt.Errorf("bluesky: profile for %q has no did", actor)
	}
	return prof.DID, nil
}

type rawProfile struct {
	DID            string `json:"did"`
	Handle         string `json:"handle"`
	DisplayName    string `json:"displayName"`
	Description    string `json:"description"`
	FollowersCount int    `json:"followersCount"`
	FollowsCount   int    `json:"followsCount"`
	PostsCount     int    `json:"postsCount"`
}

func (se *session) getProfile(ctx context.Context, actor string) (rawProfile, error) {
	query := url.Values{"actor": {actor}}
	body, err := se.call(ctx, http.MethodGet, "app.bsky.actor.getProfile", query, nil)
	if err != nil {
		return rawProfile{}, err
	}
	var prof rawProfile
	if err := json.Unmarshal(body, &prof); err != nil {
		return rawProfile{}, fmt.Errorf("bluesky: decode profile: %w", err)
	}
	return prof, nil
}
