//go:build e2e

// Real-API e2e for HubSpot: verify the OAuth-bound portal, then exercise a
// contact lifecycle that archives every record it creates.
package hubspot_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/e2e"
)

// hubSpotAccount carries the stable portal identity returned by account.
type hubSpotAccount struct {
	PortalID int64 `json:"portalId"`
}

// hubSpotContact carries the stable fields checked across the write chain.
type hubSpotContact struct {
	ID         string                   `json:"id"`
	Properties hubSpotContactProperties `json:"properties"`
	Archived   bool                     `json:"archived"`
}

// hubSpotContactProperties is the fixed property subset used by the test.
type hubSpotContactProperties struct {
	Email     string `json:"email"`
	FirstName string `json:"firstname"`
	LastName  string `json:"lastname"`
}

// TestE2EAccountRead proves the OAuth token reaches the HubSpot portal that
// supplied it rather than only satisfying local credential injection.
func TestE2EAccountRead(t *testing.T) {
	out := mustRunHubSpot(t, "account", "--json")
	var account hubSpotAccount
	decodeHubSpotJSON(t, out, &account)
	if account.PortalID == 0 {
		t.Fatalf("account returned no portal identity:\n%s", out)
	}
}

// TestE2EContactClosedLoop catches broken contact write verbs and cleanup by
// exercising create, get, update, archive, then get-after-archive.
func TestE2EContactClosedLoop(t *testing.T) {
	prefix := e2e.Prefix()
	email := prefix + "hubspot-contact@example.com"
	updatedLastName := prefix + "updated"
	contactID := ""
	archived := false

	// Archive a created contact when a later assertion stops the test early.
	defer func() {
		if contactID == "" || archived {
			return
		}
		_, stderr, exit := e2e.RunToolWithStderr(t, "hubspot", "", "contact", "delete", contactID, "--json")
		if exit != 0 {
			t.Errorf("cleanup contact %s: exit = %d\nstderr: %s", contactID, exit, stderr)
		}
	}()

	// Create an isolated contact whose run-scoped email identifies it.
	out := mustRunHubSpot(t, "contact", "create", "--prop", "email="+email, "--prop", "firstname=AnyCLI", "--prop", "lastname=E2E", "--json")
	created := decodeHubSpotContact(t, "contact create", out)
	contactID = created.ID
	if created.Properties.Email != email || created.Archived {
		t.Fatalf("contact create returned unexpected data: %+v", created)
	}

	// Read the contact back to verify creation was durably visible.
	out = mustRunHubSpot(t, "contact", "get", contactID, "--properties", "email,firstname,lastname", "--json")
	if got := decodeHubSpotContact(t, "contact get after create", out); got.ID != contactID || got.Properties.Email != email {
		t.Fatalf("contact get after create returned unexpected data: %+v", got)
	}

	// Update the last name and confirm the mutation through a fresh read.
	mustRunHubSpot(t, "contact", "update", contactID, "--prop", "lastname="+updatedLastName, "--json")
	out = mustRunHubSpot(t, "contact", "get", contactID, "--properties", "email,lastname", "--json")
	if got := decodeHubSpotContact(t, "contact get after update", out); got.ID != contactID || got.Properties.LastName != updatedLastName {
		t.Fatalf("contact get after update returned unexpected data: %+v", got)
	}

	// Archive the contact and mark the deferred cleanup complete.
	mustRunHubSpot(t, "contact", "delete", contactID, "--json")
	archived = true

	// An archived HubSpot contact must no longer be returned by the live-record path.
	out, stderr, exit := e2e.RunToolWithStderr(t, "hubspot", "", "contact", "get", contactID, "--json")
	if exit == 0 || !strings.Contains(stderr, `"status":404`) {
		t.Fatalf("contact get after archive: exit = %d, want API 404\nstderr: %s\nstdout: %s", exit, stderr, out)
	}
}

// mustRunHubSpot executes one HubSpot command and requires a successful exit.
func mustRunHubSpot(t *testing.T, args ...string) string {
	t.Helper()
	out, stderr, exit := e2e.RunToolWithStderr(t, "hubspot", "", args...)
	if exit != 0 {
		t.Fatalf("%s: exit = %d\nstderr: %s\nstdout: %s", strings.Join(args, " "), exit, stderr, out)
	}
	return out
}

// decodeHubSpotJSON decodes one provider response into its fixed response type.
func decodeHubSpotJSON(t *testing.T, out string, dst any) {
	t.Helper()
	if err := json.Unmarshal([]byte(out), dst); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
}

// decodeHubSpotContact parses one fixed contact response and requires an id.
func decodeHubSpotContact(t *testing.T, step, out string) hubSpotContact {
	t.Helper()
	var contact hubSpotContact
	decodeHubSpotJSON(t, out, &contact)
	if contact.ID == "" {
		t.Fatalf("%s returned a contact without an id:\n%s", step, out)
	}
	return contact
}
