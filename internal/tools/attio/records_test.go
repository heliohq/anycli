package attio

import (
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestRecordSearchDefaultsObjectsAndRequestAs(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /v2/objects/records/search": okData(`[]`),
	})
	defer srv.Close()

	_, errStr, exit := runService(t, srv, "record", "search", "--query", "Acme")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", exit, errStr)
	}
	req := findReq(reqs, http.MethodPost, "/v2/objects/records/search")
	if req == nil {
		t.Fatal("no search request recorded")
	}
	body := bodyMap(t, req.Body)
	if body["query"] != "Acme" {
		t.Errorf("query = %v, want Acme", body["query"])
	}
	objects := toStrings(body["objects"])
	if !reflect.DeepEqual(objects, []string{"people", "companies"}) {
		t.Errorf("objects default = %v, want [people companies]", objects)
	}
	ra, _ := body["request_as"].(map[string]any)
	if ra["type"] != "workspace" {
		t.Errorf("request_as = %v, want {type:workspace}", body["request_as"])
	}
	if _, hasLimit := body["limit"]; hasLimit {
		t.Error("limit must be omitted when --limit not passed")
	}
}

func TestRecordSearchExplicitObjectsAndLimit(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /v2/objects/records/search": okData(`[]`),
	})
	defer srv.Close()

	_, _, exit := runService(t, srv, "record", "search", "--query", "x",
		"--objects", "deals, custom_obj ", "--limit", "5")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	body := bodyMap(t, findReq(reqs, http.MethodPost, "/v2/objects/records/search").Body)
	if got := toStrings(body["objects"]); !reflect.DeepEqual(got, []string{"deals", "custom_obj"}) {
		t.Errorf("objects = %v, want [deals custom_obj] (trimmed)", got)
	}
	if body["limit"].(float64) != 5 {
		t.Errorf("limit = %v, want 5", body["limit"])
	}
}

func TestRecordSearchRequestAsMemberUUIDVsEmail(t *testing.T) {
	cases := []struct {
		name    string
		member  string
		wantKey string
	}{
		{"uuid", "50cf242c-7fa3-4cad-87d0-75b1af71c57b", "workspace_member_id"},
		{"email", "alice@attio.com", "email_address"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var reqs []capturedRequest
			srv := newMux(t, &reqs, map[string]stub{"POST /v2/objects/records/search": okData(`[]`)})
			defer srv.Close()

			_, _, exit := runService(t, srv, "record", "search", "--query", "x", "--request-as-member", tc.member)
			if exit != 0 {
				t.Fatalf("exit = %d, want 0", exit)
			}
			ra := bodyMap(t, findReq(reqs, http.MethodPost, "/v2/objects/records/search").Body)["request_as"].(map[string]any)
			if ra["type"] != "workspace-member" {
				t.Errorf("type = %v, want workspace-member", ra["type"])
			}
			if ra[tc.wantKey] != tc.member {
				t.Errorf("%s = %v, want %s", tc.wantKey, ra[tc.wantKey], tc.member)
			}
		})
	}
}

func TestRecordQueryBodyPaginationInBody(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /v2/objects/people/records/query": okData(`[]`),
	})
	defer srv.Close()

	_, errStr, exit := runService(t, srv, "record", "query", "people",
		"--filter", `{"name":"Ada"}`, "--sorts", `[{"attribute":"name","direction":"asc"}]`,
		"--limit", "10", "--offset", "20")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", exit, errStr)
	}
	body := bodyMap(t, findReq(reqs, http.MethodPost, "/v2/objects/people/records/query").Body)
	if f, _ := body["filter"].(map[string]any); f["name"] != "Ada" {
		t.Errorf("filter = %v, want {name:Ada}", body["filter"])
	}
	if _, ok := body["sorts"].([]any); !ok {
		t.Errorf("sorts = %v, want array", body["sorts"])
	}
	if body["limit"].(float64) != 10 || body["offset"].(float64) != 20 {
		t.Errorf("limit/offset = %v/%v, want 10/20", body["limit"], body["offset"])
	}
}

func TestRecordCreateDataValuesEnvelope(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /v2/objects/people/records": okData(`{"id":{"record_id":"r-1"},"record_text":"Ada"}`),
	})
	defer srv.Close()

	out, errStr, exit := runService(t, srv, "record", "create", "people", "--values", `{"name":"Ada"}`)
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", exit, errStr)
	}
	data := dataMap(t, findReq(reqs, http.MethodPost, "/v2/objects/people/records").Body)
	values, ok := data["values"].(map[string]any)
	if !ok || values["name"] != "Ada" {
		t.Errorf("data.values = %v, want {name:Ada}", data["values"])
	}
	if !strings.Contains(out, "r-1") {
		t.Errorf("summary missing record id: %q", out)
	}
}

func TestRecordUpdateDefaultsToPutOverwrite(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"PUT /v2/objects/people/records/r-1": okData(`{"id":{"record_id":"r-1"}}`),
	})
	defer srv.Close()

	_, errStr, exit := runService(t, srv, "record", "update", "people", "r-1", "--values", `{"tags":["a"]}`)
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", exit, errStr)
	}
	req := findReq(reqs, http.MethodPut, "/v2/objects/people/records/r-1")
	if req == nil {
		t.Fatal("default update must use PUT (overwrite), no PUT recorded")
	}
	if findReq(reqs, http.MethodPatch, "/v2/objects/people/records/r-1") != nil {
		t.Error("default update must not use PATCH")
	}
	data := dataMap(t, req.Body)
	if _, ok := data["values"]; !ok {
		t.Errorf("PUT body must carry data.values, got %s", req.Body)
	}
}

func TestRecordUpdateAppendUsesPatch(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"PATCH /v2/objects/people/records/r-1": okData(`{"id":{"record_id":"r-1"}}`),
	})
	defer srv.Close()

	_, errStr, exit := runService(t, srv, "record", "update", "people", "r-1", "--values", `{"tags":["b"]}`, "--append")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", exit, errStr)
	}
	if findReq(reqs, http.MethodPatch, "/v2/objects/people/records/r-1") == nil {
		t.Fatal("--append must use PATCH, no PATCH recorded")
	}
	if findReq(reqs, http.MethodPut, "/v2/objects/people/records/r-1") != nil {
		t.Error("--append must not use PUT")
	}
}

func TestRecordUpsertMatchingAttributeQueryParam(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"PUT /v2/objects/people/records": okData(`{"id":{"record_id":"r-1"}}`),
	})
	defer srv.Close()

	_, errStr, exit := runService(t, srv, "record", "upsert", "people",
		"--values", `{"email_addresses":["a@b.com"]}`, "--match", "email_addresses")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", exit, errStr)
	}
	req := findReq(reqs, http.MethodPut, "/v2/objects/people/records")
	if req == nil {
		t.Fatal("no PUT upsert recorded")
	}
	if got := req.Query.Get("matching_attribute"); got != "email_addresses" {
		t.Errorf("matching_attribute = %q, want email_addresses", got)
	}
	if _, ok := dataMap(t, req.Body)["values"]; !ok {
		t.Errorf("upsert body must carry data.values, got %s", req.Body)
	}
}

func TestRecordCreateMissingValuesIsUsageExit2(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	_, _, exit := runService(t, srv, "record", "create", "people")
	if exit != 2 {
		t.Errorf("exit = %d, want 2 for missing --values", exit)
	}
	if len(reqs) != 0 {
		t.Errorf("must not reach API without --values, got %d requests", len(reqs))
	}
}

func TestRecordGetAndDeletePaths(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/objects/companies/records/c-1":    okData(`{"id":{"record_id":"c-1"}}`),
		"DELETE /v2/objects/companies/records/c-1": okData(`{}`),
	})
	defer srv.Close()

	if _, _, exit := runService(t, srv, "record", "get", "companies", "c-1"); exit != 0 {
		t.Fatalf("get exit = %d, want 0", exit)
	}
	if _, _, exit := runService(t, srv, "record", "delete", "companies", "c-1"); exit != 0 {
		t.Fatalf("delete exit = %d, want 0", exit)
	}
	if findReq(reqs, http.MethodGet, "/v2/objects/companies/records/c-1") == nil {
		t.Error("record get path wrong")
	}
	if findReq(reqs, http.MethodDelete, "/v2/objects/companies/records/c-1") == nil {
		t.Error("record delete path wrong")
	}
}

// --- helpers --------------------------------------------------------------

func toStrings(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
