package youtube

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestChannelsGet_PartDefaultAndOverride(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /youtube/v3/channels": {http.StatusOK, `{"items":[{"id":"UC1","snippet":{"title":"Helio"},"statistics":{"subscriberCount":"10","viewCount":"99","videoCount":"3"}}]}`},
	})
	// default part
	f.runOK(t, "channels", "get", "--mine")
	got := f.last(t, "GET", "/youtube/v3/channels")
	if !strings.Contains(got.Query, "part="+strings.ReplaceAll(channelsGetPart, ",", "%2C")) {
		t.Errorf("query = %q, want default part %q", got.Query, channelsGetPart)
	}
	if !strings.Contains(got.Query, "mine=true") {
		t.Errorf("query = %q, want mine=true", got.Query)
	}
	// override part
	f.runOK(t, "channels", "get", "--id", "UC1,UC2", "--part", "snippet")
	got = f.last(t, "GET", "/youtube/v3/channels")
	if !strings.Contains(got.Query, "part=snippet") || strings.Contains(got.Query, "statistics") {
		t.Errorf("query = %q, want overridden part=snippet only", got.Query)
	}
	if !strings.Contains(got.Query, "id=UC1%2CUC2") {
		t.Errorf("query = %q, want id list", got.Query)
	}
}

func TestChannelsGet_HumanSummary(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /youtube/v3/channels": {http.StatusOK, `{"items":[{"id":"UC1","snippet":{"title":"Helio"},"statistics":{"subscriberCount":"1234","viewCount":"99","videoCount":"3"}}]}`},
	})
	out := f.runOK(t, "channels", "get", "--mine")
	if !strings.Contains(out, "Helio") || !strings.Contains(out, "1234 subs") || !strings.Contains(out, "UC1") {
		t.Errorf("human output = %q, want title + subs + id", out)
	}
}

func TestSearch_FlattensIdsAnd100UnitVerb(t *testing.T) {
	body := `{"items":[
		{"kind":"youtube#searchResult","etag":"e","id":{"kind":"youtube#video","videoId":"vid123"},"snippet":{"title":"A Video"}},
		{"kind":"youtube#searchResult","etag":"e","id":{"kind":"youtube#channel","channelId":"chan456"},"snippet":{"title":"A Channel"}}
	],"nextPageToken":"NEXT"}`
	f := newFixture(t, map[string]route{
		"GET /youtube/v3/search": {http.StatusOK, body},
	})
	out := f.runOK(t, "search", "--query", "hello", "--type", "video", "--max", "3", "--json")
	var parsed struct {
		Items []struct {
			ID      string          `json:"id"`
			Kind    string          `json:"kind"`
			Snippet json.RawMessage `json:"snippet"`
		} `json:"items"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("--json output is not valid JSON: %v (%q)", err, out)
	}
	if len(parsed.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(parsed.Items))
	}
	if parsed.Items[0].ID != "vid123" || parsed.Items[0].Kind != "youtube#video" {
		t.Errorf("item0 = %+v, want flattened video id", parsed.Items[0])
	}
	if parsed.Items[1].ID != "chan456" {
		t.Errorf("item1 id = %q, want flattened channel id", parsed.Items[1].ID)
	}
	if parsed.NextPageToken != "NEXT" {
		t.Errorf("nextPageToken = %q, want NEXT", parsed.NextPageToken)
	}
	got := f.last(t, "GET", "/youtube/v3/search")
	for _, want := range []string{"q=hello", "type=video", "maxResults=3", "part=snippet"} {
		if !strings.Contains(got.Query, want) {
			t.Errorf("query = %q, want %q", got.Query, want)
		}
	}
}

func TestVideosMine_TwoStepResolve_NeverSearch(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /youtube/v3/channels":      {http.StatusOK, `{"items":[{"contentDetails":{"relatedPlaylists":{"uploads":"UU_uploads"}}}]}`},
		"GET /youtube/v3/playlistItems": {http.StatusOK, `{"items":[{"id":"pi1","snippet":{"title":"My Upload","resourceId":{"videoId":"v9"}}}],"nextPageToken":"P2"}`},
	})
	out := f.runOK(t, "videos", "mine", "--max", "10", "--json")

	ch := f.last(t, "GET", "/youtube/v3/channels")
	if !strings.Contains(ch.Query, "mine=true") || !strings.Contains(ch.Query, "part=contentDetails") {
		t.Errorf("channels query = %q, want mine=true + part=contentDetails", ch.Query)
	}
	pi := f.last(t, "GET", "/youtube/v3/playlistItems")
	if !strings.Contains(pi.Query, "playlistId=UU_uploads") {
		t.Errorf("playlistItems query = %q, want the uploads playlist id", pi.Query)
	}
	if f.count("GET", "/youtube/v3/search") != 0 {
		t.Error("videos mine must NOT call search.list")
	}
	if !strings.Contains(out, "v9") {
		t.Errorf("--json output = %q, want the upload video id", out)
	}
}

func TestVideosUpdate_ReadModifyWrite(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /youtube/v3/videos": {http.StatusOK, `{"items":[{"snippet":{"title":"Old Title","description":"old","categoryId":"22","tags":["keep"]}}]}`},
		"PUT /youtube/v3/videos": {http.StatusOK, `{"id":"v1","snippet":{"title":"New Title","categoryId":"22"}}`},
	})
	f.runOK(t, "videos", "update", "--id", "v1", "--title", "New Title", "--json")

	if f.count("GET", "/youtube/v3/videos") != 1 {
		t.Fatalf("expected a GET before the PUT (read-modify-write)")
	}
	put := f.last(t, "PUT", "/youtube/v3/videos")
	var payload struct {
		ID      string `json:"id"`
		Snippet struct {
			Title      string `json:"title"`
			CategoryID string `json:"categoryId"`
		} `json:"snippet"`
	}
	if err := json.Unmarshal(put.Body, &payload); err != nil {
		t.Fatalf("PUT body not JSON: %v", err)
	}
	if payload.ID != "v1" || payload.Snippet.Title != "New Title" {
		t.Errorf("PUT payload = %+v, want merged id + new title", payload)
	}
	if payload.Snippet.CategoryID != "22" {
		t.Errorf("PUT payload dropped categoryId (%q); RMW must preserve untouched required fields", payload.Snippet.CategoryID)
	}
	if !strings.Contains(put.Query, "part=snippet") || strings.Contains(put.Query, "status") {
		t.Errorf("PUT query = %q, want part=snippet only (no --privacy given)", put.Query)
	}
}

func TestVideosUpdate_PrivacyAddsStatusPart(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /youtube/v3/videos": {http.StatusOK, `{"items":[{"snippet":{"title":"T","categoryId":"22"},"status":{"privacyStatus":"public","license":"youtube"}}]}`},
		"PUT /youtube/v3/videos": {http.StatusOK, `{"id":"v1"}`},
	})
	f.runOK(t, "videos", "update", "--id", "v1", "--privacy", "unlisted", "--json")
	put := f.last(t, "PUT", "/youtube/v3/videos")
	if !strings.Contains(put.Query, "part=snippet%2Cstatus") {
		t.Errorf("PUT query = %q, want part=snippet,status when --privacy set", put.Query)
	}
	var payload struct {
		Status struct {
			Privacy string `json:"privacyStatus"`
			License string `json:"license"`
		} `json:"status"`
	}
	_ = json.Unmarshal(put.Body, &payload)
	if payload.Status.Privacy != "unlisted" {
		t.Errorf("privacyStatus = %q, want unlisted", payload.Status.Privacy)
	}
	if payload.Status.License != "youtube" {
		t.Errorf("status.license = %q, RMW must preserve untouched status fields", payload.Status.License)
	}
}

func TestVideosRate_EmptyBodyMutationOK(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /youtube/v3/videos/rate": {http.StatusNoContent, ``},
	})
	out := f.runOK(t, "videos", "rate", "--id", "v1", "--rating", "like", "--json")
	assertOKEnvelope(t, out, "v1")
	got := f.last(t, "POST", "/youtube/v3/videos/rate")
	if !strings.Contains(got.Query, "id=v1") || !strings.Contains(got.Query, "rating=like") {
		t.Errorf("query = %q, want id + rating", got.Query)
	}
}

func TestPlaylistRoundTrip_CreateAddRemoveDelete(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /youtube/v3/playlists":       {http.StatusOK, `{"id":"PL1","snippet":{"title":"Scratch"}}`},
		"POST /youtube/v3/playlistItems":   {http.StatusOK, `{"id":"PI1","snippet":{"resourceId":{"videoId":"v1"}}}`},
		"DELETE /youtube/v3/playlistItems": {http.StatusNoContent, ``},
		"DELETE /youtube/v3/playlists":     {http.StatusNoContent, ``},
	})
	out := f.runOK(t, "playlists", "create", "--title", "Scratch", "--privacy", "private", "--json")
	if !strings.Contains(out, "PL1") {
		t.Errorf("create output = %q, want new playlist id", out)
	}
	create := f.last(t, "POST", "/youtube/v3/playlists")
	if !strings.Contains(create.Query, "part=snippet%2Cstatus") {
		t.Errorf("create query = %q, want part=snippet,status", create.Query)
	}

	f.runOK(t, "playlist-items", "add", "--playlist", "PL1", "--video", "v1", "--json")
	add := f.last(t, "POST", "/youtube/v3/playlistItems")
	var addBody struct {
		Snippet struct {
			PlaylistID string `json:"playlistId"`
			ResourceID struct {
				VideoID string `json:"videoId"`
			} `json:"resourceId"`
		} `json:"snippet"`
	}
	_ = json.Unmarshal(add.Body, &addBody)
	if addBody.Snippet.PlaylistID != "PL1" || addBody.Snippet.ResourceID.VideoID != "v1" {
		t.Errorf("add body = %+v, want playlist + video resourceId", addBody)
	}

	rmOut := f.runOK(t, "playlist-items", "remove", "--id", "PI1", "--json")
	assertOKEnvelope(t, rmOut, "PI1")
	delOut := f.runOK(t, "playlists", "delete", "--id", "PL1", "--json")
	assertOKEnvelope(t, delOut, "PL1")
}

func TestCommentsList_PagingAndReplies(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /youtube/v3/commentThreads": {http.StatusOK, `{"items":[{"id":"ct1","snippet":{"topLevelComment":{"snippet":{"authorDisplayName":"Alice","textDisplay":"nice video"}},"totalReplyCount":2}}],"nextPageToken":"CT2"}`},
	})
	out := f.runOK(t, "comments", "list", "--video", "v1", "--order", "time", "--max", "20", "--page", "PREV", "--json")
	got := f.last(t, "GET", "/youtube/v3/commentThreads")
	for _, want := range []string{"videoId=v1", "order=time", "maxResults=20", "pageToken=PREV", "part=snippet%2Creplies"} {
		if !strings.Contains(got.Query, want) {
			t.Errorf("query = %q, want %q", got.Query, want)
		}
	}
	var parsed struct {
		Items         []json.RawMessage `json:"items"`
		NextPageToken string            `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("--json not valid: %v", err)
	}
	if parsed.NextPageToken != "CT2" {
		t.Errorf("nextPageToken = %q, want CT2 echoed", parsed.NextPageToken)
	}
}

func TestCommentsReply_PostsTextOriginal(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /youtube/v3/comments": {http.StatusOK, `{"id":"c9","snippet":{"textDisplay":"thanks!"}}`},
	})
	f.runOK(t, "comments", "reply", "--parent", "ct1", "--text", "thanks!", "--json")
	got := f.last(t, "POST", "/youtube/v3/comments")
	var body struct {
		Snippet struct {
			ParentID     string `json:"parentId"`
			TextOriginal string `json:"textOriginal"`
		} `json:"snippet"`
	}
	_ = json.Unmarshal(got.Body, &body)
	if body.Snippet.ParentID != "ct1" || body.Snippet.TextOriginal != "thanks!" {
		t.Errorf("reply body = %+v, want parentId + textOriginal", body)
	}
}

func TestCommentsModerate_RejectWithBan(t *testing.T) {
	f := newFixture(t, map[string]route{
		"POST /youtube/v3/comments/setModerationStatus": {http.StatusNoContent, ``},
	})
	out := f.runOK(t, "comments", "moderate", "--id", "c1", "--status", "rejected", "--ban-author", "--json")
	assertOKEnvelope(t, out, "c1")
	got := f.last(t, "POST", "/youtube/v3/comments/setModerationStatus")
	for _, want := range []string{"id=c1", "moderationStatus=rejected", "banAuthor=true"} {
		if !strings.Contains(got.Query, want) {
			t.Errorf("query = %q, want %q", got.Query, want)
		}
	}
}

func TestSubscriptionsList_MineRequired(t *testing.T) {
	f := newFixture(t, map[string]route{
		"GET /youtube/v3/subscriptions": {http.StatusOK, `{"items":[{"snippet":{"title":"Some Channel","resourceId":{"channelId":"UCabc"}}}]}`},
	})
	out := f.runOK(t, "subscriptions", "list", "--mine")
	if !strings.Contains(out, "UCabc") || !strings.Contains(out, "Some Channel") {
		t.Errorf("human output = %q, want channel id + title", out)
	}
}

// assertOKEnvelope verifies the standard empty-body mutation envelope.
func assertOKEnvelope(t *testing.T, out, wantID string) {
	t.Helper()
	var env struct {
		OK bool   `json:"ok"`
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &env); err != nil {
		t.Fatalf("output is not a JSON ok-envelope: %v (%q)", err, out)
	}
	if !env.OK || env.ID != wantID {
		t.Errorf("envelope = %+v, want {ok:true, id:%q}", env, wantID)
	}
}
