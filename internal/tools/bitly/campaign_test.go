package bitly

import (
	"net/http"
	"testing"
)

func TestCampaignList_AutoResolvesGroupQuery(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newMultiServer(t, map[string]routeHandler{
		"/user":      {status: http.StatusOK, response: `{"default_group_guid":"Bg-auto"}`},
		"/campaigns": {status: http.StatusOK, response: `{"campaigns":[]}`},
	}, captured)
	defer srv.Close()

	code, _, _ := run(t, srv, "campaign", "list")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	req := captured["/campaigns"]
	if req.Method != http.MethodGet {
		t.Errorf("method = %s, want GET", req.Method)
	}
	if q := parseQuery(t, req.Query); q.Get("group_guid") != "Bg-auto" {
		t.Errorf("group_guid = %q, want auto-resolved", q.Get("group_guid"))
	}
}

func TestCampaignGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"guid":"Ca1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "campaign", "get", "--campaign", "Ca1")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/campaigns/Ca1" {
		t.Errorf("path = %q", got.Path)
	}
}

func TestCampaignCreate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"guid":"Ca1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "campaign", "create", "--group", "Bg1", "--name", "Launch",
		"--description", "desc", "--channel-guids", "Ch1", "--channel-guids", "Ch2")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/campaigns" {
		t.Errorf("request = %s %s, want POST /campaigns", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["name"] != "Launch" || body["group_guid"] != "Bg1" {
		t.Errorf("body = %v", body)
	}
	if guids, ok := body["channel_guids"].([]any); !ok || len(guids) != 2 {
		t.Errorf("channel_guids = %v", body["channel_guids"])
	}
}

func TestChannelCreate_PairedBitlinks(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"guid":"Ch1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "campaign", "channel", "create", "--group", "Bg1", "--name", "Ch",
		"--bitlink", "bit.ly/2ab", "--campaign", "Ca1", "--bitlink", "bit.ly/2cd", "--guid", "Ch-client")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/channels" {
		t.Errorf("request = %s %s, want POST /channels", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["name"] != "Ch" || body["guid"] != "Ch-client" {
		t.Errorf("body = %v", body)
	}
	bitlinks, ok := body["bitlinks"].([]any)
	if !ok || len(bitlinks) != 2 {
		t.Fatalf("bitlinks = %v, want two entries", body["bitlinks"])
	}
	first := bitlinks[0].(map[string]any)
	if first["bitlink_id"] != "bit.ly/2ab" || first["campaign_guid"] != "Ca1" {
		t.Errorf("first bitlink entry = %v, want paired campaign", first)
	}
	second := bitlinks[1].(map[string]any)
	if second["bitlink_id"] != "bit.ly/2cd" {
		t.Errorf("second bitlink_id = %v", second["bitlink_id"])
	}
	if _, ok := second["campaign_guid"]; ok {
		t.Errorf("second entry should have no campaign_guid, got %v", second)
	}
}

func TestChannelUpdate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"guid":"Ch1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "campaign", "channel", "update", "--channel", "Ch1", "--name", "Renamed")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPatch || got.Path != "/channels/Ch1" {
		t.Errorf("request = %s %s, want PATCH /channels/Ch1", got.Method, got.Path)
	}
	if body := decodeBody(t, got.Body); body["name"] != "Renamed" {
		t.Errorf("name = %v", body["name"])
	}
}
