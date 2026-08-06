//go:build e2e

// Real-API e2e for Zoho CRM: an authenticated identity smoke test plus a
// self-cleaning Lead create/read/delete chain.
package zohocrm_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/e2e"
)

// crmUsersResponse is the provider envelope returned by `user me`.
type crmUsersResponse struct {
	Users []crmUser `json:"users"`
}

// crmUser carries the stable identity fields checked by the smoke test.
type crmUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

// crmMutationResponse is the provider envelope returned by create/delete.
type crmMutationResponse struct {
	Data []crmMutationResult `json:"data"`
}

// crmMutationResult carries one record mutation result from Zoho CRM.
type crmMutationResult struct {
	Code    string             `json:"code"`
	Status  string             `json:"status"`
	Details crmMutationDetails `json:"details"`
}

// crmMutationDetails carries the record id assigned by Zoho CRM.
type crmMutationDetails struct {
	ID string `json:"id"`
}

// crmRecordResponse is the provider envelope returned by record get.
type crmRecordResponse struct {
	Data []crmRecord `json:"data"`
}

// crmRecord carries the fields checked after creating a Lead.
type crmRecord struct {
	ID       string `json:"id"`
	LastName string `json:"Last_Name"`
}

// crmErrorEnvelope carries the machine-readable failure emitted under --json
// so deletion verification can distinguish not-found from unrelated errors.
type crmErrorEnvelope struct {
	Error crmError `json:"error"`
}

// crmError carries the provider error classification used after delete.
type crmError struct {
	Message string `json:"message"`
	Kind    string `json:"kind"`
	Status  int    `json:"status"`
}

// TestE2EIdentity confirms the credential reaches the authenticated CRM user.
func TestE2EIdentity(t *testing.T) {
	out := mustRunCRM(t, "user", "me", "--json")
	var response crmUsersResponse
	decodeCRMJSON(t, out, &response)
	if len(response.Users) == 0 || response.Users[0].ID == "" {
		t.Fatalf("user me returned no authenticated user:\n%s", out)
	}
}

// TestE2ELeadClosedLoop covers a reversible CRM write without retaining test data.
func TestE2ELeadClosedLoop(t *testing.T) {
	name := e2e.Prefix() + "lead"
	payload := fmt.Sprintf(`{"Last_Name":%q,"Company":"AnyCLI E2E"}`, name)
	out := mustRunCRM(t, "record", "create", "--module", "Leads", "--data", payload, "--no-triggers", "--json")

	var created crmMutationResponse
	decodeCRMJSON(t, out, &created)
	if len(created.Data) == 0 || created.Data[0].Code != "SUCCESS" || created.Data[0].Details.ID == "" {
		t.Fatalf("create did not return a successful Lead id:\n%s", out)
	}
	recordID := created.Data[0].Details.ID

	// Cleanup still runs when a later assertion fails, preventing test residue.
	deleted := false
	t.Cleanup(func() {
		if deleted {
			return
		}
		if _, exit := e2e.RunTool(t, "zoho-crm", "", "record", "delete", "--module", "Leads", "--id", recordID, "--json"); exit != 0 {
			t.Errorf("cleanup delete for Lead %s exited %d", recordID, exit)
		}
	})

	out = mustRunCRM(t, "record", "get", "--module", "Leads", "--id", recordID, "--fields", "id,Last_Name", "--json")
	var fetched crmRecordResponse
	decodeCRMJSON(t, out, &fetched)
	if len(fetched.Data) == 0 || fetched.Data[0].ID != recordID || fetched.Data[0].LastName != name {
		t.Fatalf("get did not return the created Lead %s with name %q:\n%s", recordID, name, out)
	}

	out = mustRunCRM(t, "record", "delete", "--module", "Leads", "--id", recordID, "--json")
	var removed crmMutationResponse
	decodeCRMJSON(t, out, &removed)
	if len(removed.Data) == 0 || removed.Data[0].Code != "SUCCESS" {
		t.Fatalf("delete did not report success for Lead %s:\n%s", recordID, out)
	}
	deleted = true

	// Zoho may report a deleted record as either an explicit not-found API error
	// or a successful empty response; unrelated failures must not prove cleanup.
	out, stderr, exit := e2e.RunToolWithStderr(t, "zoho-crm", "", "record", "get", "--module", "Leads", "--id", recordID, "--json")
	if exit == 0 {
		if strings.TrimSpace(out) == "" {
			return
		}
		var afterDelete crmRecordResponse
		decodeCRMJSON(t, out, &afterDelete)
		if len(afterDelete.Data) != 0 {
			t.Fatalf("get after delete still returned Lead %s:\n%s", recordID, out)
		}
		return
	}
	var notFound crmErrorEnvelope
	decodeCRMJSON(t, stderr, &notFound)
	if notFound.Error.Kind != "api" || notFound.Error.Status != 404 || !strings.Contains(notFound.Error.Message, "RESOURCE_NOT_FOUND") {
		t.Fatalf("get after delete returned an unexpected error for Lead %s:\n%s", recordID, stderr)
	}
}

// mustRunCRM executes one CRM command and requires a successful exit.
func mustRunCRM(t *testing.T, args ...string) string {
	t.Helper()
	out, stderr, exit := e2e.RunToolWithStderr(t, "zoho-crm", "", args...)
	if exit != 0 {
		t.Fatalf("%s: exit = %d\nstderr: %s\nstdout: %s", strings.Join(args, " "), exit, stderr, out)
	}
	return out
}

// decodeCRMJSON decodes one provider response into its fixed response type.
func decodeCRMJSON(t *testing.T, out string, dst any) {
	t.Helper()
	if err := json.Unmarshal([]byte(out), dst); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
}
