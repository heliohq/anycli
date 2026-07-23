package outreach

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestProspectCreateBuildsJSONAPIBody(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusCreated, `{"data":{"type":"prospect","id":"55"}}`)
	})
	defer server.Close()

	code, stdout, stderr := run(t, server, fullEnv(),
		"prospect", "create",
		"--first-name", "Sally", "--last-name", "Smith", "--email", "s@x.com",
		"--account-id", "5", "--owner-id", "9", "--stage-id", "3",
		"--attr", "title=VP Sales", "--attr", "score=42")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/prospects" || got.ContentType != mediaType {
		t.Fatalf("request = %s %s %q", got.Method, got.Path, got.ContentType)
	}

	var body struct {
		Data struct {
			Type       string `json:"type"`
			ID         string `json:"id"`
			Attributes struct {
				FirstName string   `json:"firstName"`
				LastName  string   `json:"lastName"`
				Emails    []string `json:"emails"`
				Title     string   `json:"title"`
				Score     float64  `json:"score"`
			} `json:"attributes"`
			Relationships map[string]struct {
				Data linkage `json:"data"`
			} `json:"relationships"`
		} `json:"data"`
	}
	if err := json.Unmarshal(got.Body, &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Data.Type != "prospect" || body.Data.ID != "" {
		t.Fatalf("create body must carry type and omit id: %s", got.Body)
	}
	a := body.Data.Attributes
	if a.FirstName != "Sally" || a.LastName != "Smith" || a.Title != "VP Sales" || a.Score != 42 {
		t.Fatalf("attributes = %+v", a)
	}
	if len(a.Emails) != 1 || a.Emails[0] != "s@x.com" {
		t.Fatalf("emails = %v", a.Emails)
	}
	if body.Data.Relationships["account"].Data.Type != "account" || body.Data.Relationships["account"].Data.ID != "5" {
		t.Fatalf("account rel = %+v", body.Data.Relationships["account"])
	}
	if body.Data.Relationships["owner"].Data.Type != "user" || body.Data.Relationships["owner"].Data.ID != "9" {
		t.Fatalf("owner rel = %+v (must be type user)", body.Data.Relationships["owner"])
	}
	if body.Data.Relationships["stage"].Data.Type != "stage" {
		t.Fatalf("stage rel = %+v", body.Data.Relationships["stage"])
	}
	if stdout != `{"id":"55","type":"prospect"}`+"\n" {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestProspectUpdateCarriesTypeAndID(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, `{"data":{"type":"prospect","id":"55"}}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "prospect", "update", "55", "--title", "Director")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got.Method != http.MethodPatch || got.Path != "/prospects/55" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	var body struct {
		Data struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(got.Body, &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Data.Type != "prospect" || body.Data.ID != "55" {
		t.Fatalf("update body must carry type and id: %s", got.Body)
	}
}

func TestAccountCreateAndListFilters(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		var got capturedRequest
		server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			got = captureRequest(t, r)
			jsonResponse(w, http.StatusCreated, `{"data":{"type":"account","id":"7"}}`)
		})
		defer server.Close()
		code, _, stderr := run(t, server, fullEnv(), "account", "create", "--name", "Acme", "--domain", "acme.com", "--owner-id", "9")
		if code != 0 {
			t.Fatalf("exit code = %d, stderr = %q", code, stderr)
		}
		if got.Path != "/accounts" || got.Method != http.MethodPost {
			t.Fatalf("request = %s %s", got.Method, got.Path)
		}
	})

	t.Run("list filters", func(t *testing.T) {
		var got capturedRequest
		server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			got = captureRequest(t, r)
			jsonResponse(w, http.StatusOK, `{"data":[]}`)
		})
		defer server.Close()
		code, _, stderr := run(t, server, fullEnv(), "account", "list", "--q", "acme", "--domain", "acme.com")
		if code != 0 {
			t.Fatalf("exit code = %d, stderr = %q", code, stderr)
		}
		for _, want := range []string{"filter[q]=acme", "filter[domain]=acme.com"} {
			if !strings.Contains(got.RawQuery, want) {
				t.Fatalf("query = %q, missing %q", got.RawQuery, want)
			}
		}
	})
}
