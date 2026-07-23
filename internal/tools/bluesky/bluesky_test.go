package bluesky

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestExecuteRequiresCredentials(t *testing.T) {
	// A value with no colon (missing app password) is malformed.
	s := newStub(t)
	result, _, stderr := runStub(t, s, map[string]string{EnvCredentials: "alice.bsky.social"}, "whoami")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, "BLUESKY_CREDENTIALS must be set") {
		t.Fatalf("stderr = %q", stderr)
	}
	if s.count("com.atproto.server.createSession") != 0 {
		t.Fatal("createSession should not be called without credentials")
	}
}

func TestSplitCredentials(t *testing.T) {
	cases := []struct {
		in         string
		identifier string
		password   string
		ok         bool
	}{
		{"alice.bsky.social:xxxx-xxxx-xxxx-xxxx", "alice.bsky.social", "xxxx-xxxx-xxxx-xxxx", true},
		{"alice@example.com:pw-1234", "alice@example.com", "pw-1234", true},
		{"  alice.bsky.social : pw  ", "alice.bsky.social", "pw", true},
		{"no-colon", "", "", false},
		{":only-password", "", "", false},
		{"only-identifier:", "", "", false},
		{"", "", "", false},
	}
	for _, tc := range cases {
		id, pw, ok := splitCredentials(tc.in)
		if ok != tc.ok || id != tc.identifier || pw != tc.password {
			t.Errorf("splitCredentials(%q) = (%q,%q,%t), want (%q,%q,%t)", tc.in, id, pw, ok, tc.identifier, tc.password, tc.ok)
		}
	}
}

func TestWhoamiOpensSessionAndReadsProfile(t *testing.T) {
	s := newStub(t)
	s.on("app.bsky.actor.getProfile", ok(`{"did":"did:plc:alice","handle":"alice.bsky.social","displayName":"Alice","description":"hi","followersCount":10,"followsCount":5,"postsCount":3}`))

	result, stdout, stderr := runStub(t, s, fullEnv(), "whoami", "--json")
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, stderr = %q", result.ExitCode, stderr)
	}

	// createSession sent the app-password body.
	sessReq := s.last("com.atproto.server.createSession")
	if sessReq.Method != http.MethodPost {
		t.Fatalf("createSession method = %s", sessReq.Method)
	}
	var sessBody map[string]string
	if err := json.Unmarshal(sessReq.Body, &sessBody); err != nil {
		t.Fatalf("session body: %v", err)
	}
	if sessBody["identifier"] != testHandle || sessBody["password"] != "app-pass-1234" {
		t.Fatalf("session body = %v", sessBody)
	}

	// getProfile ran against the connected DID with a Bearer access token.
	profReq := s.last("app.bsky.actor.getProfile")
	if profReq.Auth != "Bearer "+testAccessJwt {
		t.Fatalf("getProfile auth = %q", profReq.Auth)
	}
	if !strings.Contains(profReq.Query, "actor=did%3Aplc%3Aalice") {
		t.Fatalf("getProfile query = %q", profReq.Query)
	}

	out := decode(t, stdout)
	if out["handle"] != "alice.bsky.social" || out["display_name"] != "Alice" {
		t.Fatalf("output = %v", out)
	}
	if out["followers_count"].(float64) != 10 {
		t.Fatalf("followers_count = %v", out["followers_count"])
	}
}

func TestPostCreateComputesFacetsAndReturnsRef(t *testing.T) {
	restore := nowRFC3339
	nowRFC3339 = func() string { return "2026-07-22T00:00:00Z" }
	defer func() { nowRFC3339 = restore }()

	s := newStub(t)
	s.on("com.atproto.repo.createRecord", ok(`{"uri":"at://did:plc:alice/app.bsky.feed.post/abc","cid":"bafypost"}`))

	result, stdout, stderr := runStub(t, s, fullEnv(), "post", "create",
		"--text", "Check https://example.com and #golang", "--lang", "en", "--json")
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, stderr = %q", result.ExitCode, stderr)
	}

	req := s.last("com.atproto.repo.createRecord")
	if req.Auth != "Bearer "+testAccessJwt {
		t.Fatalf("createRecord auth = %q", req.Auth)
	}
	var payload struct {
		Repo       string `json:"repo"`
		Collection string `json:"collection"`
		Record     struct {
			Type      string   `json:"$type"`
			Text      string   `json:"text"`
			CreatedAt string   `json:"createdAt"`
			Langs     []string `json:"langs"`
			Facets    []facet  `json:"facets"`
		} `json:"record"`
	}
	if err := json.Unmarshal(req.Body, &payload); err != nil {
		t.Fatalf("decode createRecord body: %v", err)
	}
	if payload.Repo != testDID || payload.Collection != collectionPost {
		t.Fatalf("repo/collection = %q/%q", payload.Repo, payload.Collection)
	}
	if payload.Record.Type != collectionPost || payload.Record.CreatedAt != "2026-07-22T00:00:00Z" {
		t.Fatalf("record = %+v", payload.Record)
	}
	if len(payload.Record.Langs) != 1 || payload.Record.Langs[0] != "en" {
		t.Fatalf("langs = %v", payload.Record.Langs)
	}
	// Facets: a link at bytes 6-25 and a hashtag at bytes 30-37.
	if len(payload.Record.Facets) != 2 {
		t.Fatalf("facets = %+v", payload.Record.Facets)
	}
	link := payload.Record.Facets[0]
	if link.Index.ByteStart != 6 || link.Index.ByteEnd != 25 ||
		link.Features[0].Type != facetLink || link.Features[0].URI != "https://example.com" {
		t.Fatalf("link facet = %+v", link)
	}
	tag := payload.Record.Facets[1]
	if tag.Index.ByteStart != 30 || tag.Index.ByteEnd != 37 ||
		tag.Features[0].Type != facetTag || tag.Features[0].Tag != "golang" {
		t.Fatalf("tag facet = %+v", tag)
	}

	out := decode(t, stdout)
	if out["uri"] != "at://did:plc:alice/app.bsky.feed.post/abc" || out["cid"] != "bafypost" || out["handle"] != testHandle {
		t.Fatalf("output = %v", out)
	}
}

func TestPostCreateWithImageUploadsBlobAndEmbeds(t *testing.T) {
	pngBytes := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0}
	dir := t.TempDir()
	path := dir + "/cat.png"
	if err := writeFile(t, path, pngBytes); err != nil {
		t.Fatal(err)
	}

	s := newStub(t)
	s.on("com.atproto.repo.uploadBlob", ok(`{"blob":{"$type":"blob","ref":{"$link":"bafyblob"},"mimeType":"image/png","size":12}}`))
	s.on("com.atproto.repo.createRecord", ok(`{"uri":"at://did:plc:alice/app.bsky.feed.post/img","cid":"bafyimg"}`))

	result, _, stderr := runStub(t, s, fullEnv(), "post", "create",
		"--text", "look", "--image", path, "--alt", "a cat", "--json")
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, stderr = %q", result.ExitCode, stderr)
	}

	// uploadBlob sent raw PNG bytes with the sniffed content type.
	blobReq := s.last("com.atproto.repo.uploadBlob")
	if blobReq.ContentType != "image/png" {
		t.Fatalf("uploadBlob content-type = %q", blobReq.ContentType)
	}
	if string(blobReq.Body) != string(pngBytes) {
		t.Fatalf("uploadBlob body = %v", blobReq.Body)
	}

	// createRecord embeds the blob under app.bsky.embed.images with alt text.
	req := s.last("com.atproto.repo.createRecord")
	var payload struct {
		Record struct {
			Embed struct {
				Type   string `json:"$type"`
				Images []struct {
					Alt   string `json:"alt"`
					Image struct {
						Ref struct {
							Link string `json:"$link"`
						} `json:"ref"`
					} `json:"image"`
				} `json:"images"`
			} `json:"embed"`
		} `json:"record"`
	}
	if err := json.Unmarshal(req.Body, &payload); err != nil {
		t.Fatalf("decode createRecord: %v", err)
	}
	if payload.Record.Embed.Type != "app.bsky.embed.images" {
		t.Fatalf("embed type = %q", payload.Record.Embed.Type)
	}
	if len(payload.Record.Embed.Images) != 1 || payload.Record.Embed.Images[0].Alt != "a cat" ||
		payload.Record.Embed.Images[0].Image.Ref.Link != "bafyblob" {
		t.Fatalf("embed images = %+v", payload.Record.Embed.Images)
	}
}

func TestPostDeleteParsesATURI(t *testing.T) {
	s := newStub(t)
	s.on("com.atproto.repo.deleteRecord", ok(`{}`))

	uri := "at://did:plc:alice/app.bsky.feed.post/abc"
	result, stdout, stderr := runStub(t, s, fullEnv(), "post", "delete", "--uri", uri, "--json")
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, stderr = %q", result.ExitCode, stderr)
	}
	var payload map[string]string
	if err := json.Unmarshal(s.last("com.atproto.repo.deleteRecord").Body, &payload); err != nil {
		t.Fatalf("decode deleteRecord: %v", err)
	}
	if payload["repo"] != "did:plc:alice" || payload["collection"] != collectionPost || payload["rkey"] != "abc" {
		t.Fatalf("deleteRecord body = %v", payload)
	}
	out := decode(t, stdout)
	if out["uri"] != uri || out["deleted"] != "true" {
		t.Fatalf("output = %v", out)
	}
}

func TestPostDeleteRejectsBadURI(t *testing.T) {
	s := newStub(t)
	result, _, stderr := runStub(t, s, fullEnv(), "post", "delete", "--uri", "https://not-at-uri", "--json")
	if result.ExitCode == 0 {
		t.Fatal("bad at:// URI should fail")
	}
	if !strings.Contains(stderr, "at:// URI") {
		t.Fatalf("stderr = %q", stderr)
	}
	if s.count("com.atproto.repo.deleteRecord") != 0 {
		t.Fatal("deleteRecord should not be called for an invalid URI")
	}
}

func TestTimelineShapesPosts(t *testing.T) {
	s := newStub(t)
	s.on("app.bsky.feed.getTimeline", ok(`{"feed":[{"post":{"uri":"at://p/1","cid":"c1","author":{"did":"did:plc:bob","handle":"bob.bsky.social","displayName":"Bob"},"record":{"text":"hello","createdAt":"2026-07-01T00:00:00Z"},"replyCount":2,"repostCount":3,"likeCount":4}}],"cursor":"next-1"}`))

	result, stdout, stderr := runStub(t, s, fullEnv(), "timeline", "--limit", "25", "--json")
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, stderr = %q", result.ExitCode, stderr)
	}
	req := s.last("app.bsky.feed.getTimeline")
	if req.Method != http.MethodGet || req.Auth != "Bearer "+testAccessJwt {
		t.Fatalf("timeline request = %+v", req)
	}
	var out struct {
		Posts []struct {
			URI    string `json:"uri"`
			Text   string `json:"text"`
			Author struct {
				Handle string `json:"handle"`
			} `json:"author"`
			LikeCount int `json:"like_count"`
		} `json:"posts"`
		Cursor string `json:"cursor"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Cursor != "next-1" || len(out.Posts) != 1 {
		t.Fatalf("out = %+v", out)
	}
	p := out.Posts[0]
	if p.URI != "at://p/1" || p.Text != "hello" || p.Author.Handle != "bob.bsky.social" || p.LikeCount != 4 {
		t.Fatalf("post = %+v", p)
	}
}

func TestSearchPostsShapesResults(t *testing.T) {
	s := newStub(t)
	s.on("app.bsky.feed.searchPosts", ok(`{"posts":[{"uri":"at://p/2","cid":"c2","author":{"did":"d","handle":"h","displayName":""},"record":{"text":"found","createdAt":""},"replyCount":0,"repostCount":0,"likeCount":1}],"cursor":""}`))

	result, stdout, _ := runStub(t, s, fullEnv(), "search", "posts", "--q", "golang", "--json")
	if result.ExitCode != 0 {
		t.Fatalf("exit = %d", result.ExitCode)
	}
	if !strings.Contains(s.last("app.bsky.feed.searchPosts").Query, "q=golang") {
		t.Fatalf("query = %q", s.last("app.bsky.feed.searchPosts").Query)
	}
	if !strings.Contains(stdout, `"text":"found"`) {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestLikeCreatesRecordWithSubject(t *testing.T) {
	s := newStub(t)
	s.on("com.atproto.repo.createRecord", ok(`{"uri":"at://did:plc:alice/app.bsky.feed.like/lk","cid":"bafylike"}`))

	result, stdout, stderr := runStub(t, s, fullEnv(), "like", "--uri", "at://did:plc:bob/app.bsky.feed.post/xyz", "--cid", "bafytarget", "--json")
	if result.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", result.ExitCode, stderr)
	}
	var payload struct {
		Collection string `json:"collection"`
		Record     struct {
			Type    string    `json:"$type"`
			Subject recordRef `json:"subject"`
		} `json:"record"`
	}
	if err := json.Unmarshal(s.last("com.atproto.repo.createRecord").Body, &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Collection != collectionLike || payload.Record.Type != collectionLike {
		t.Fatalf("payload = %+v", payload)
	}
	if payload.Record.Subject.URI != "at://did:plc:bob/app.bsky.feed.post/xyz" || payload.Record.Subject.CID != "bafytarget" {
		t.Fatalf("subject = %+v", payload.Record.Subject)
	}
	_ = decode(t, stdout)
}

func TestFollowResolvesActorDID(t *testing.T) {
	s := newStub(t)
	s.on("app.bsky.actor.getProfile", ok(`{"did":"did:plc:bob","handle":"bob.bsky.social"}`))
	s.on("com.atproto.repo.createRecord", ok(`{"uri":"at://did:plc:alice/app.bsky.graph.follow/fl","cid":"bafyfollow"}`))

	result, stdout, stderr := runStub(t, s, fullEnv(), "follow", "--actor", "bob.bsky.social", "--json")
	if result.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", result.ExitCode, stderr)
	}
	var payload struct {
		Record struct {
			Subject string `json:"subject"`
		} `json:"record"`
	}
	if err := json.Unmarshal(s.last("com.atproto.repo.createRecord").Body, &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Record.Subject != "did:plc:bob" {
		t.Fatalf("follow subject = %q", payload.Record.Subject)
	}
	out := decode(t, stdout)
	if out["subject"] != "did:plc:bob" {
		t.Fatalf("output = %v", out)
	}
}

func TestErrorEnvelopeAndExitCode(t *testing.T) {
	s := newStub(t)
	s.on("app.bsky.feed.getTimeline", fail(http.StatusBadRequest, `{"error":"InvalidRequest","message":"bad cursor"}`))

	result, _, stderr := runStub(t, s, fullEnv(), "timeline", "--json")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Fatal("InvalidRequest should not reject the credential")
	}
	if !strings.Contains(stderr, "InvalidRequest") && !strings.Contains(stderr, "bad cursor") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestCredentialRejectionClassification(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		body         string
		wantRejected bool
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, body: `{"error":"AuthenticationRequired","message":"no"}`, wantRejected: true},
		{name: "invalid token", status: http.StatusBadRequest, body: `{"error":"InvalidToken","message":"no"}`, wantRejected: true},
		{name: "auth factor required", status: http.StatusUnauthorized, body: `{"error":"AuthFactorTokenRequired"}`, wantRejected: true},
		{name: "rate limited", status: http.StatusTooManyRequests, body: `{"error":"RateLimitExceeded"}`, wantRejected: false},
		{name: "server error", status: http.StatusInternalServerError, body: `{"error":"InternalServerError"}`, wantRejected: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newStub(t)
			// Fail at createSession so the rejection surfaces on the login itself.
			s.on("com.atproto.server.createSession", fail(tc.status, tc.body))
			result, _, _ := runStub(t, s, fullEnv(), "timeline", "--json")
			if result.CredentialRejected != tc.wantRejected {
				t.Fatalf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
		})
	}
}

func TestExpiredTokenReestablishesSessionOnce(t *testing.T) {
	s := newStub(t)
	s.on("app.bsky.feed.getTimeline",
		fail(http.StatusBadRequest, `{"error":"ExpiredToken","message":"token expired"}`),
		ok(`{"feed":[],"cursor":""}`),
	)

	result, _, stderr := runStub(t, s, fullEnv(), "timeline", "--json")
	if result.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", result.ExitCode, stderr)
	}
	if got := s.count("com.atproto.server.createSession"); got != 2 {
		t.Fatalf("createSession count = %d, want 2 (initial + re-establish)", got)
	}
	if got := s.count("app.bsky.feed.getTimeline"); got != 2 {
		t.Fatalf("getTimeline count = %d, want 2 (fail + retry)", got)
	}
}

func TestUnknownSubcommandFails(t *testing.T) {
	s := newStub(t)
	result, _, stderr := runStub(t, s, fullEnv(), "not-a-command")
	if result.ExitCode == 0 {
		t.Fatal("unknown subcommand should fail")
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Fatalf("stderr = %q", stderr)
	}
}
