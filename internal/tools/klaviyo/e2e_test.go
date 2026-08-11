//go:build e2e

// Real-API e2e for Klaviyo: verify the OAuth-bound account, then follow list
// discovery into a concrete list read when the account has one.
package klaviyo_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/e2e"
)

// klaviyoCollectionResponse is the JSON:API collection envelope returned by
// account and list discovery commands.
type klaviyoCollectionResponse struct {
	Data []klaviyoResource `json:"data"`
}

// klaviyoResourceResponse is the JSON:API envelope returned by resource reads.
type klaviyoResourceResponse struct {
	Data klaviyoResource `json:"data"`
}

// klaviyoResource carries the stable identity shared by collection and get
// responses without coupling the test to mutable marketing attributes.
type klaviyoResource struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// TestE2EAccountRead proves the OAuth token resolves the single Klaviyo
// account used by the provider identity probe.
func TestE2EAccountRead(t *testing.T) {
	out := mustRunKlaviyo(t, "account", "get", "--json")
	var accounts klaviyoCollectionResponse
	decodeKlaviyoJSON(t, out, &accounts)
	if len(accounts.Data) != 1 || accounts.Data[0].ID == "" || accounts.Data[0].Type == "" {
		t.Fatalf("account get returned an invalid account collection:\n%s", out)
	}
}

// TestE2EListRead verifies collection pagination and, when available, follows
// a discovered list id through the resource endpoint without creating data.
func TestE2EListRead(t *testing.T) {
	out := mustRunKlaviyo(t, "list", "list", "--page-size", "1", "--json")
	var lists klaviyoCollectionResponse
	decodeKlaviyoJSON(t, out, &lists)
	if len(lists.Data) == 0 {
		return
	}
	discovered := lists.Data[0]
	if discovered.ID == "" || discovered.Type == "" {
		t.Fatalf("list discovery returned a resource without identity:\n%s", out)
	}

	// The follow-up request proves a real id from pagination is accepted by the
	// dedicated resource path with the same OAuth credential.
	out = mustRunKlaviyo(t, "list", "get", discovered.ID, "--json")
	var fetched klaviyoResourceResponse
	decodeKlaviyoJSON(t, out, &fetched)
	if fetched.Data.ID != discovered.ID || fetched.Data.Type != discovered.Type {
		t.Fatalf("list get did not return discovered list %s:\n%s", discovered.ID, out)
	}
}

// mustRunKlaviyo executes one Klaviyo command and requires a successful exit.
func mustRunKlaviyo(t *testing.T, args ...string) string {
	t.Helper()
	out, stderr, exit := e2e.RunToolWithStderr(t, "klaviyo", "", args...)
	if exit != 0 {
		t.Fatalf("%s: exit = %d\nstderr: %s\nstdout: %s", strings.Join(args, " "), exit, stderr, out)
	}
	return out
}

// decodeKlaviyoJSON decodes one provider response into its fixed response type.
func decodeKlaviyoJSON(t *testing.T, out string, dst any) {
	t.Helper()
	if err := json.Unmarshal([]byte(out), dst); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
}
