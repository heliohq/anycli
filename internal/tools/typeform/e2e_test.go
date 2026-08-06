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

// typeformErrorEnvelope carries the machine-readable failure emitted under
// --json so cleanup verification can distinguish not-found from other errors.
type typeformErrorEnvelope struct {
	Error typeformError `json:"error"`
}

// typeformError carries the provider error classification used after delete.
type typeformError struct {
	Message string `json:"message"`
	Kind    string `json:"kind"`
	Status  int    `json:"status"`
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
	if created.ID == "" {
		t.Fatalf("form create returned no form id:\n%s", out)
	}

	// Register cleanup as soon as deletion is possible so metadata assertions
	// cannot leave a successfully created form behind.
	deleted := false
	t.Cleanup(func() {
		if deleted {
			return
		}
		if _, exit := e2e.RunTool(t, "typeform", "", "form", "delete", created.ID); exit != 0 {
			t.Errorf("cleanup delete for form %s exited %d", created.ID, exit)
		}
	})
	if created.Title != name {
		t.Fatalf("form create returned title %q, want %q:\n%s", created.Title, name, out)
	}

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

	// Only Typeform's explicit not-found response proves deletion; unrelated
	// credential, rate-limit, transport, or server failures must fail the test.
	out, stderr, exit := e2e.RunToolWithStderr(t, "typeform", "", "--json", "form", "get", created.ID)
	if exit == 0 {
		t.Fatalf("form get after delete succeeded for %s:\n%s", created.ID, out)
	}
	var notFound typeformErrorEnvelope
	decodeTypeformJSON(t, stderr, &notFound)
	if notFound.Error.Kind != "api" || notFound.Error.Status != 404 || !strings.Contains(notFound.Error.Message, "FORM_NOT_FOUND") {
		t.Fatalf("form get after delete returned an unexpected error for %s:\n%s", created.ID, stderr)
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
