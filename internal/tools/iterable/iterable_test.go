package iterable

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// --- credential split ------------------------------------------------------

func TestParseCredential(t *testing.T) {
	cases := []struct {
		name    string
		secret  string
		wantKey string
		wantURL string
		wantErr bool
	}{
		{name: "us two-part", secret: "us:KEY123", wantKey: "KEY123", wantURL: baseURLUS},
		{name: "eu two-part", secret: "eu:KEY456", wantKey: "KEY456", wantURL: baseURLEU},
		{name: "us aliased three-part", secret: "us:staging:KEY789", wantKey: "KEY789", wantURL: baseURLUS},
		{name: "eu aliased three-part", secret: "eu:prod:KEYABC", wantKey: "KEYABC", wantURL: baseURLEU},
		{name: "unknown region", secret: "apac:KEY", wantErr: true},
		{name: "one part", secret: "KEYONLY", wantErr: true},
		{name: "four parts", secret: "us:a:b:KEY", wantErr: true},
		{name: "empty key two-part", secret: "us:", wantErr: true},
		{name: "empty key three-part", secret: "us:staging:", wantErr: true},
		{name: "empty secret", secret: "", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cred, err := parseCredential(tc.secret)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseCredential(%q) = %+v, want error", tc.secret, cred)
				}
				var ue *usageError
				if !asUsageError(err, &ue) {
					t.Fatalf("parseCredential(%q) error = %T, want *usageError", tc.secret, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseCredential(%q) unexpected error: %v", tc.secret, err)
			}
			if cred.apiKey != tc.wantKey {
				t.Errorf("apiKey = %q, want %q", cred.apiKey, tc.wantKey)
			}
			if cred.baseURL != tc.wantURL {
				t.Errorf("baseURL = %q, want %q", cred.baseURL, tc.wantURL)
			}
		})
	}
}

// asUsageError is a tiny errors.As wrapper kept local to avoid importing errors
// in every case.
func asUsageError(err error, target **usageError) bool {
	if ue, ok := err.(*usageError); ok {
		*target = ue
		return true
	}
	return false
}

// --- auth header injection -------------------------------------------------

func TestApiKeyHeaderInjectedNotBearer(t *testing.T) {
	got := &capturedRequest{}
	srv := newServer(t, http.StatusOK, `{"lists":[]}`, got)
	defer srv.Close()

	res, _, errOut := runWithKey(t, srv, "us:KEY123", "list", "list")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr=%s", res.ExitCode, errOut)
	}
	if got.APIKey != "KEY123" {
		t.Errorf("Api-Key header = %q, want KEY123", got.APIKey)
	}
	if got.Auth != "" {
		t.Errorf("Authorization header = %q, want empty (Iterable uses Api-Key, not Bearer)", got.Auth)
	}
	if got.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", got.Accept)
	}
}

func TestAliasSegmentIgnoredByService(t *testing.T) {
	got := &capturedRequest{}
	srv := newServer(t, http.StatusOK, `{"lists":[]}`, got)
	defer srv.Close()

	// Three-part aliased secret: the middle "staging" must not reach the API;
	// only the last segment is the key.
	if _, _, errOut := runWithKey(t, srv, "us:staging:KEY789", "list", "list"); errOut != "" {
		t.Fatalf("unexpected stderr: %s", errOut)
	}
	if got.APIKey != "KEY789" {
		t.Errorf("Api-Key = %q, want KEY789 (alias segment must be dropped)", got.APIKey)
	}
}

// --- request shapes per verb ----------------------------------------------

func TestUserGetByEmail(t *testing.T) {
	got := &capturedRequest{}
	srv := newServer(t, http.StatusOK, `{"user":{}}`, got)
	defer srv.Close()

	runWithKey(t, srv, "us:K", "user", "get", "--email", "a@b.com")
	if got.Method != http.MethodGet || got.Path != "/api/users/a@b.com" {
		t.Errorf("got %s %s, want GET /api/users/a@b.com", got.Method, got.Path)
	}
}

func TestUserGetByUserID(t *testing.T) {
	got := &capturedRequest{}
	srv := newServer(t, http.StatusOK, `{"user":{}}`, got)
	defer srv.Close()

	runWithKey(t, srv, "us:K", "user", "get", "--user-id", "u-42")
	if got.Method != http.MethodGet || got.Path != "/api/users/byUserId/u-42" {
		t.Errorf("got %s %s, want GET /api/users/byUserId/u-42", got.Method, got.Path)
	}
}

func TestUserGetRejectsBothSelectors(t *testing.T) {
	got := &capturedRequest{}
	srv := newServer(t, http.StatusOK, `{}`, got)
	defer srv.Close()

	res, _, _ := runWithKey(t, srv, "us:K", "user", "get", "--email", "a@b.com", "--user-id", "u1")
	if res.ExitCode != 2 {
		t.Errorf("exit = %d, want 2 (mutually exclusive selectors)", res.ExitCode)
	}
	if got.Method != "" {
		t.Errorf("server was called (%s %s); expected fail before HTTP", got.Method, got.Path)
	}
}

func TestUserUpdatePostsBody(t *testing.T) {
	got := &capturedRequest{}
	srv := newServer(t, http.StatusOK, `{"code":"Success","msg":"ok"}`, got)
	defer srv.Close()

	runWithKey(t, srv, "us:K", "user", "update", "--body", `{"email":"a@b.com","dataFields":{"firstName":"Ada"}}`)
	if got.Method != http.MethodPost || got.Path != "/api/users/update" {
		t.Fatalf("got %s %s, want POST /api/users/update", got.Method, got.Path)
	}
	if got.Content != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got.Content)
	}
	body := decodeBody(t, got.Body)
	if body["email"] != "a@b.com" {
		t.Errorf("body email = %v, want a@b.com", body["email"])
	}
}

func TestUserDelete(t *testing.T) {
	got := &capturedRequest{}
	srv := newServer(t, http.StatusOK, `{"code":"Success"}`, got)
	defer srv.Close()

	runWithKey(t, srv, "us:K", "user", "delete", "--email", "a@b.com")
	if got.Method != http.MethodDelete || got.Path != "/api/users/a@b.com" {
		t.Errorf("got %s %s, want DELETE /api/users/a@b.com", got.Method, got.Path)
	}
}

func TestUserFields(t *testing.T) {
	got := &capturedRequest{}
	srv := newServer(t, http.StatusOK, `{"fields":{}}`, got)
	defer srv.Close()

	runWithKey(t, srv, "us:K", "user", "fields")
	if got.Method != http.MethodGet || got.Path != "/api/users/getFields" {
		t.Errorf("got %s %s, want GET /api/users/getFields", got.Method, got.Path)
	}
}

func TestEventTrack(t *testing.T) {
	got := &capturedRequest{}
	srv := newServer(t, http.StatusOK, `{"code":"Success"}`, got)
	defer srv.Close()

	runWithKey(t, srv, "us:K", "event", "track", "--body", `{"email":"a@b.com","eventName":"signup"}`)
	if got.Method != http.MethodPost || got.Path != "/api/events/track" {
		t.Fatalf("got %s %s, want POST /api/events/track", got.Method, got.Path)
	}
	if decodeBody(t, got.Body)["eventName"] != "signup" {
		t.Errorf("body eventName = %v, want signup", decodeBody(t, got.Body)["eventName"])
	}
}

func TestEventList(t *testing.T) {
	got := &capturedRequest{}
	srv := newServer(t, http.StatusOK, `{"events":[]}`, got)
	defer srv.Close()

	runWithKey(t, srv, "us:K", "event", "list", "--email", "a@b.com", "--limit", "5")
	if got.Method != http.MethodGet || got.Path != "/api/events/a@b.com" {
		t.Errorf("got %s %s, want GET /api/events/a@b.com", got.Method, got.Path)
	}
	if got.Query != "limit=5" {
		t.Errorf("query = %q, want limit=5", got.Query)
	}
}

func TestListVerbs(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantMethod string
		wantPath   string
		wantQuery  string
	}{
		{"list", []string{"list", "list"}, http.MethodGet, "/api/lists", ""},
		{"subscribe", []string{"list", "subscribe", "--body", `{"listId":1,"subscribers":[{"email":"a@b.com"}]}`}, http.MethodPost, "/api/lists/subscribe", ""},
		{"unsubscribe", []string{"list", "unsubscribe", "--body", `{"listId":1,"subscribers":[{"email":"a@b.com"}]}`}, http.MethodPost, "/api/lists/unsubscribe", ""},
		{"users", []string{"list", "users", "--list-id", "42"}, http.MethodGet, "/api/lists/getUsers", "listId=42"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := &capturedRequest{}
			srv := newServer(t, http.StatusOK, `{"code":"Success","lists":[]}`, got)
			defer srv.Close()
			runWithKey(t, srv, "us:K", tc.args...)
			if got.Method != tc.wantMethod || got.Path != tc.wantPath {
				t.Errorf("got %s %s, want %s %s", got.Method, got.Path, tc.wantMethod, tc.wantPath)
			}
			if got.Query != tc.wantQuery {
				t.Errorf("query = %q, want %q", got.Query, tc.wantQuery)
			}
		})
	}
}

func TestCampaignAndTemplateAndCatalog(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantMethod string
		wantPath   string
		wantQuery  string
	}{
		{"campaign list", []string{"campaign", "list"}, http.MethodGet, "/api/campaigns", ""},
		{"campaign metrics", []string{"campaign", "metrics", "--campaign-id", "7"}, http.MethodGet, "/api/campaigns/metrics", "campaignId=7"},
		{"template list", []string{"template", "list"}, http.MethodGet, "/api/templates", ""},
		{"catalog list", []string{"catalog", "list"}, http.MethodGet, "/api/catalogs", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := &capturedRequest{}
			srv := newServer(t, http.StatusOK, `{"campaigns":[]}`, got)
			defer srv.Close()
			runWithKey(t, srv, "us:K", tc.args...)
			if got.Method != tc.wantMethod || got.Path != tc.wantPath {
				t.Errorf("got %s %s, want %s %s", got.Method, got.Path, tc.wantMethod, tc.wantPath)
			}
			if got.Query != tc.wantQuery {
				t.Errorf("query = %q, want %q", got.Query, tc.wantQuery)
			}
		})
	}
}

func TestEmailSend(t *testing.T) {
	got := &capturedRequest{}
	srv := newServer(t, http.StatusOK, `{"code":"Success"}`, got)
	defer srv.Close()

	runWithKey(t, srv, "us:K", "email", "send", "--body", `{"campaignId":7,"recipientEmail":"a@b.com"}`)
	if got.Method != http.MethodPost || got.Path != "/api/email/target" {
		t.Errorf("got %s %s, want POST /api/email/target", got.Method, got.Path)
	}
}

// --- error rendering -------------------------------------------------------

func TestBadCredentialExitTwoTextAndJSON(t *testing.T) {
	// text
	res, _, errOut := runNoServer(t, "not-a-valid-secret", "list", "list")
	if res.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2", res.ExitCode)
	}
	if !strings.Contains(errOut, "<region>") {
		t.Errorf("stderr = %q, want format guidance", errOut)
	}

	// json
	res, _, errOut = runNoServer(t, "not-a-valid-secret", "--json", "list", "list")
	if res.ExitCode != 2 {
		t.Fatalf("json exit = %d, want 2", res.ExitCode)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(errOut)), &env); err != nil {
		t.Fatalf("stderr not JSON envelope: %v (%s)", err, errOut)
	}
	if env.Error.Code != "usage" {
		t.Errorf("error.code = %q, want usage", env.Error.Code)
	}
}

func TestAPIErrorExitOneAndRejectsCredentialOn401(t *testing.T) {
	got := &capturedRequest{}
	srv := newServer(t, http.StatusUnauthorized, `{"code":"BadApiKey","msg":"invalid key"}`, got)
	defer srv.Close()

	res, _, errOut := runWithKey(t, srv, "us:K", "list", "list")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !res.CredentialRejected {
		t.Errorf("CredentialRejected = false, want true on 401")
	}
	if !strings.Contains(errOut, "invalid key") {
		t.Errorf("stderr = %q, want provider message", errOut)
	}
}

func TestAPIErrorJSONEnvelopeCarriesStatus(t *testing.T) {
	got := &capturedRequest{}
	srv := newServer(t, http.StatusUnprocessableEntity, `{"code":"InvalidEmailAddressError","msg":"bad email"}`, got)
	defer srv.Close()

	res, _, errOut := runWithKey(t, srv, "us:K", "--json", "list", "list")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	var env struct {
		Error struct {
			Code   string `json:"code"`
			Status int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(errOut)), &env); err != nil {
		t.Fatalf("stderr not JSON: %v (%s)", err, errOut)
	}
	if env.Error.Code != "api" || env.Error.Status != http.StatusUnprocessableEntity {
		t.Errorf("envelope = %+v, want code=api status=422", env.Error)
	}
}

func TestNonSuccessCodeOn200IsExitOne(t *testing.T) {
	got := &capturedRequest{}
	// HTTP 200 but application-level non-Success code (Iterable's write dialect).
	srv := newServer(t, http.StatusOK, `{"code":"BadParams","msg":"missing listId"}`, got)
	defer srv.Close()

	res, out, _ := runWithKey(t, srv, "us:K", "list", "subscribe", "--body", `{"subscribers":[]}`)
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1 (non-Success code)", res.ExitCode)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("stdout = %q, want empty on failure", out)
	}
}

func TestSuccessCodeEmitsBody(t *testing.T) {
	got := &capturedRequest{}
	srv := newServer(t, http.StatusOK, `{"code":"Success","msg":"done"}`, got)
	defer srv.Close()

	res, out, _ := runWithKey(t, srv, "us:K", "event", "track", "--body", `{"email":"a@b.com","eventName":"x"}`)
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	if !strings.Contains(out, `"code":"Success"`) {
		t.Errorf("stdout = %q, want passthrough body", out)
	}
}

func TestUnknownSubcommandExitTwo(t *testing.T) {
	got := &capturedRequest{}
	srv := newServer(t, http.StatusOK, `{}`, got)
	defer srv.Close()

	res, _, _ := runWithKey(t, srv, "us:K", "user", "frobnicate")
	if res.ExitCode != 2 {
		t.Errorf("exit = %d, want 2 for unknown subcommand", res.ExitCode)
	}
}
