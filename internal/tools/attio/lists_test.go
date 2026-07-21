package attio

import (
	"net/http"
	"testing"
)

func TestEntryQueryBodyAndPath(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /v2/lists/pipeline/entries/query": okData(`[]`),
	})
	defer srv.Close()

	_, errStr, exit := runService(t, srv, "entry", "query", "pipeline",
		"--filter", `{"stage":"Lead"}`, "--limit", "50")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", exit, errStr)
	}
	req := findReq(reqs, http.MethodPost, "/v2/lists/pipeline/entries/query")
	if req == nil {
		t.Fatal("no entry query recorded")
	}
	body := bodyMap(t, req.Body)
	if f, _ := body["filter"].(map[string]any); f["stage"] != "Lead" {
		t.Errorf("filter = %v, want {stage:Lead}", body["filter"])
	}
	if body["limit"].(float64) != 50 {
		t.Errorf("limit = %v, want 50", body["limit"])
	}
}

func TestEntryAddDataEnvelope(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /v2/lists/pipeline/entries": okData(`{"id":{"entry_id":"e-1"}}`),
	})
	defer srv.Close()

	_, errStr, exit := runService(t, srv, "entry", "add", "pipeline",
		"--parent-record", "r-1", "--parent-object", "people")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", exit, errStr)
	}
	data := dataMap(t, findReq(reqs, http.MethodPost, "/v2/lists/pipeline/entries").Body)
	if data["parent_record_id"] != "r-1" || data["parent_object"] != "people" {
		t.Errorf("data = %v, want parent_record_id=r-1 parent_object=people", data)
	}
	if _, ok := data["entry_values"].(map[string]any); !ok {
		t.Errorf("entry_values must be present (empty object when --values omitted), got %v", data["entry_values"])
	}
}

func TestEntryAddWithValues(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /v2/lists/pipeline/entries": okData(`{"id":{"entry_id":"e-1"}}`),
	})
	defer srv.Close()

	_, _, exit := runService(t, srv, "entry", "add", "pipeline",
		"--parent-record", "r-1", "--parent-object", "people", "--values", `{"stage":"Lead"}`)
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	data := dataMap(t, findReq(reqs, http.MethodPost, "/v2/lists/pipeline/entries").Body)
	ev, _ := data["entry_values"].(map[string]any)
	if ev["stage"] != "Lead" {
		t.Errorf("entry_values = %v, want {stage:Lead}", data["entry_values"])
	}
}

func TestEntryUpdateDefaultPutAppendPatchWithEntryValues(t *testing.T) {
	// Default: PUT overwrite.
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"PUT /v2/lists/pipeline/entries/e-1":   okData(`{"id":{"entry_id":"e-1"}}`),
		"PATCH /v2/lists/pipeline/entries/e-1": okData(`{"id":{"entry_id":"e-1"}}`),
	})
	defer srv.Close()

	if _, errStr, exit := runService(t, srv, "entry", "update", "pipeline", "e-1", "--values", `{"stage":"Won"}`); exit != 0 {
		t.Fatalf("default update exit = %d, want 0 (stderr=%s)", exit, errStr)
	}
	putReq := findReq(reqs, http.MethodPut, "/v2/lists/pipeline/entries/e-1")
	if putReq == nil {
		t.Fatal("default entry update must use PUT")
	}
	if _, ok := dataMap(t, putReq.Body)["entry_values"]; !ok {
		t.Errorf("entry update body must carry data.entry_values, got %s", putReq.Body)
	}

	// --append: PATCH.
	if _, _, exit := runService(t, srv, "entry", "update", "pipeline", "e-1", "--values", `{"tags":["x"]}`, "--append"); exit != 0 {
		t.Fatalf("append update exit = %d, want 0", exit)
	}
	if findReq(reqs, http.MethodPatch, "/v2/lists/pipeline/entries/e-1") == nil {
		t.Error("--append entry update must use PATCH")
	}
}

func TestEntryRemoveAndGetPaths(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/lists/pipeline/entries/e-1":    okData(`{"id":{"entry_id":"e-1"}}`),
		"DELETE /v2/lists/pipeline/entries/e-1": okData(`{}`),
	})
	defer srv.Close()

	if _, _, exit := runService(t, srv, "entry", "get", "pipeline", "e-1"); exit != 0 {
		t.Fatalf("get exit = %d, want 0", exit)
	}
	if _, _, exit := runService(t, srv, "entry", "remove", "pipeline", "e-1"); exit != 0 {
		t.Fatalf("remove exit = %d, want 0", exit)
	}
	if findReq(reqs, http.MethodDelete, "/v2/lists/pipeline/entries/e-1") == nil {
		t.Error("entry remove must DELETE the entry path")
	}
}

func TestObjectAndListDiscovery(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/objects":       okData(`[{"id":{"object_id":"o-1"},"api_slug":"people"}]`),
		"GET /v2/lists":         okData(`[{"id":{"list_id":"l-1"},"name":"Pipeline"}]`),
		"GET /v2/objects/deals": okData(`{"id":{"object_id":"o-2"},"api_slug":"deals"}`),
	})
	defer srv.Close()

	if out, _, exit := runService(t, srv, "object", "list"); exit != 0 || !containsAll(out, "o-1", "people") {
		t.Errorf("object list out=%q exit=%d", out, exit)
	}
	if out, _, exit := runService(t, srv, "list", "list"); exit != 0 || !containsAll(out, "l-1", "Pipeline") {
		t.Errorf("list list out=%q exit=%d", out, exit)
	}
	if _, _, exit := runService(t, srv, "object", "get", "deals"); exit != 0 {
		t.Errorf("object get deals exit=%d", exit)
	}
}

func TestAttributeTargetSelection(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /v2/objects/people/attributes":               okData(`[]`),
		"GET /v2/lists/pipeline/attributes":               okData(`[]`),
		"GET /v2/objects/deals/attributes/stage/statuses": okData(`[]`),
	})
	defer srv.Close()

	if _, _, exit := runService(t, srv, "attribute", "list", "--object", "people"); exit != 0 {
		t.Errorf("attribute list --object exit=%d", exit)
	}
	if _, _, exit := runService(t, srv, "attribute", "list", "--list", "pipeline"); exit != 0 {
		t.Errorf("attribute list --list exit=%d", exit)
	}
	if _, _, exit := runService(t, srv, "attribute", "statuses", "--object", "deals", "--attribute", "stage"); exit != 0 {
		t.Errorf("attribute statuses exit=%d", exit)
	}
	// Neither target → usage error, no request.
	before := len(reqs)
	if _, _, exit := runService(t, srv, "attribute", "list"); exit != 2 {
		t.Errorf("attribute list with no target: exit=%d, want 2", exit)
	}
	if len(reqs) != before {
		t.Error("missing-target attribute list must not reach the API")
	}
	// Both targets → usage error.
	if _, _, exit := runService(t, srv, "attribute", "list", "--object", "people", "--list", "pipeline"); exit != 2 {
		t.Errorf("attribute list with both targets: exit=%d, want 2", exit)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		found := false
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
