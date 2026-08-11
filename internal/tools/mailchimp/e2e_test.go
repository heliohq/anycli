//go:build e2e

// Real-API e2e for Mailchimp: verify OAuth metadata discovery, account health,
// and audience reads without creating persistent marketing data.
package mailchimp_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/e2e"
)

// mailchimpPingResponse carries the authenticated account health result.
type mailchimpPingResponse struct {
	HealthStatus string `json:"health_status"`
}

// mailchimpAudienceListResponse carries the audience collection envelope.
type mailchimpAudienceListResponse struct {
	Lists      []mailchimpAudience `json:"lists"`
	TotalItems int                 `json:"total_items"`
}

// mailchimpAudienceResponse carries one audience returned by audience get.
type mailchimpAudienceResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// mailchimpAudience carries the stable fields checked across list and get.
type mailchimpAudience struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// TestE2EPing proves the token can discover its Mailchimp data center and
// reach the authenticated Marketing API.
func TestE2EPing(t *testing.T) {
	out := mustRunMailchimp(t, "ping", "--json")
	var response mailchimpPingResponse
	decodeMailchimpJSON(t, out, &response)
	if response.HealthStatus == "" {
		t.Fatalf("ping returned no health status:\n%s", out)
	}
}

// TestE2EAudienceRead verifies the collection envelope and follows an existing
// audience into a detail read when the test account has one.
func TestE2EAudienceRead(t *testing.T) {
	out := mustRunMailchimp(t, "audience", "list", "--count", "5", "--json")
	var response mailchimpAudienceListResponse
	decodeMailchimpJSON(t, out, &response)
	if response.Lists == nil {
		t.Fatalf("audience list response omitted lists:\n%s", out)
	}
	if response.TotalItems < len(response.Lists) {
		t.Fatalf("audience list total_items=%d is smaller than returned lists=%d:\n%s", response.TotalItems, len(response.Lists), out)
	}
	if len(response.Lists) == 0 {
		return
	}

	// The follow-up call proves an id discovered through the collection can be
	// used by the detail endpoint with the same OAuth credential.
	audience := response.Lists[0]
	if audience.ID == "" || audience.Name == "" {
		t.Fatalf("audience list returned an entry without id or name:\n%s", out)
	}
	out = mustRunMailchimp(t, "audience", "get", audience.ID, "--json")
	var fetched mailchimpAudienceResponse
	decodeMailchimpJSON(t, out, &fetched)
	if fetched.ID != audience.ID || fetched.Name != audience.Name {
		t.Fatalf("audience get did not return discovered audience %s with name %q:\n%s", audience.ID, audience.Name, out)
	}
}

// mustRunMailchimp executes one Mailchimp command and requires success.
func mustRunMailchimp(t *testing.T, args ...string) string {
	t.Helper()
	out, stderr, exit := e2e.RunToolWithStderr(t, "mailchimp", "", args...)
	if exit != 0 {
		t.Fatalf("%s: exit = %d\nstderr: %s\nstdout: %s", strings.Join(args, " "), exit, stderr, out)
	}
	return out
}

// decodeMailchimpJSON decodes one provider response into its fixed type.
func decodeMailchimpJSON(t *testing.T, out string, dst any) {
	t.Helper()
	if err := json.Unmarshal([]byte(out), dst); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
}
