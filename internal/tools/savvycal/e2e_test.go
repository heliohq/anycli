//go:build e2e

// Real-API e2e for SavvyCal: authenticated identity plus the two collection
// reads that are useful even when the connected account has no scheduled data.
package savvycal_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/e2e"
)

// savvyCalAccount carries the stable identity fields returned by `me`.
type savvyCalAccount struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

// savvyCalCollection preserves the list envelope when entries is empty.
type savvyCalCollection struct {
	Entries  []json.RawMessage    `json:"entries"`
	Metadata savvyCalListMetadata `json:"metadata"`
}

// savvyCalListMetadata carries the pagination contract shared by list calls.
type savvyCalListMetadata struct {
	Limit int `json:"limit"`
}

// TestE2EIdentity confirms the credential reaches the authenticated account.
func TestE2EIdentity(t *testing.T) {
	out := mustRunSavvyCal(t, "me")
	var account savvyCalAccount
	decodeSavvyCalJSON(t, out, &account)
	if account.ID == "" || account.Email == "" || account.DisplayName == "" {
		t.Fatalf("me returned no usable account identity:\n%s", out)
	}
}

// TestE2EReadCollections covers links and events without creating provider data.
func TestE2EReadCollections(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "links", args: []string{"link", "list", "--limit", "5"}},
		{name: "events", args: []string{"event", "list", "--state", "all", "--period", "all", "--limit", "5"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := mustRunSavvyCal(t, tc.args...)
			var collection savvyCalCollection
			decodeSavvyCalJSON(t, out, &collection)
			if collection.Entries == nil || collection.Metadata.Limit != 5 {
				t.Fatalf("%s returned no usable collection envelope:\n%s", strings.Join(tc.args, " "), out)
			}
		})
	}
}

// mustRunSavvyCal executes one SavvyCal command and requires a successful exit.
func mustRunSavvyCal(t *testing.T, args ...string) string {
	t.Helper()
	out, stderr, exit := e2e.RunToolWithStderr(t, "savvycal", "", args...)
	if exit != 0 {
		t.Fatalf("%s: exit = %d\nstderr: %s\nstdout: %s", strings.Join(args, " "), exit, stderr, out)
	}
	return out
}

// decodeSavvyCalJSON decodes one provider response into its fixed response type.
func decodeSavvyCalJSON(t *testing.T, out string, dst any) {
	t.Helper()
	if err := json.Unmarshal([]byte(out), dst); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
}
