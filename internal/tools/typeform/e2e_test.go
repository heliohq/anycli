//go:build e2e

// Real-API e2e for Typeform: an authenticated account smoke test plus a
// self-cleaning form create/read/delete chain.
package typeform_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/e2e"
)

// typeformAccount carries the stable identity fields returned by `me`.
type typeformAccount struct {
	Alias    string `json:"alias"`
	Email    string `json:"email"`
	Language string `json:"language"`
}

// typeformForm carries the fields checked across create and get.
type typeformForm struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// typeformDeleteReceipt is the client-side receipt emitted after a 204 delete.
type typeformDeleteReceipt struct {
	Deleted bool   `json:"deleted"`
	FormID  string `json:"form_id"`
}

// TestE2EIdentity confirms the credential reaches the authenticated account.
func TestE2EIdentity(t *testing.T) {
	out := mustRunTypeform(t, "me")
	var account typeformAccount
	decodeTypeformJSON(t, out, &account)
	if account.Email == "" || account.Alias == "" {
		t.Fatalf("me returned no usable account identity:\n%s", out)
	}
}

// TestE2EFormClosedLoop covers a reversible form write without retaining test data.
func TestE2EFormClosedLoop(t *testing.T) {
	name := e2e.Prefix() + "form"
	definition := fmt.Sprintf(
		`{"title":%q,"settings":{"is_public":false},"fields":[{"title":"E2E question","type":"short_text"}]}`,
		name,
	)
	out := mustRunTypeform(t, "form", "create", "--definition", definition)

	var created typeformForm
	decodeTypeformJSON(t, out, &created)
	if created.ID == "" || created.Title != name {
		t.Fatalf("form create returned no matching form id/title:\n%s", out)
	}

	// Cleanup still runs when a later assertion fails, preventing test residue.
	deleted := false
	t.Cleanup(func() {
		if deleted {
			return
		}
		if _, exit := e2e.RunTool(t, "typeform", "", "form", "delete", created.ID); exit != 0 {
			t.Errorf("cleanup delete for form %s exited %d", created.ID, exit)
		}
	})

	out = mustRunTypeform(t, "form", "get", created.ID)
	var fetched typeformForm
	decodeTypeformJSON(t, out, &fetched)
	if fetched.ID != created.ID || fetched.Title != name {
		t.Fatalf("form get did not return created form %s with title %q:\n%s", created.ID, name, out)
	}

	out = mustRunTypeform(t, "form", "delete", created.ID)
	var receipt typeformDeleteReceipt
	decodeTypeformJSON(t, out, &receipt)
	if !receipt.Deleted || receipt.FormID != created.ID {
		t.Fatalf("form delete returned an invalid receipt for %s:\n%s", created.ID, out)
	}
	deleted = true

	// A successful read after deletion would prove the cleanup did not take effect.
	out, _, exit := e2e.RunToolWithStderr(t, "typeform", "", "form", "get", created.ID)
	if exit == 0 {
		t.Fatalf("form get after delete succeeded for %s:\n%s", created.ID, out)
	}
}

// mustRunTypeform executes one Typeform command and requires a successful exit.
func mustRunTypeform(t *testing.T, args ...string) string {
	t.Helper()
	out, stderr, exit := e2e.RunToolWithStderr(t, "typeform", "", args...)
	if exit != 0 {
		t.Fatalf("%s: exit = %d\nstderr: %s\nstdout: %s", strings.Join(args, " "), exit, stderr, out)
	}
	return out
}

// decodeTypeformJSON decodes one provider response into its fixed response type.
func decodeTypeformJSON(t *testing.T, out string, dst any) {
	t.Helper()
	if err := json.Unmarshal([]byte(out), dst); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
}
