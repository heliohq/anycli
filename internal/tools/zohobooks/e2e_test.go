//go:build e2e

// Real-API e2e for Zoho Books: discover the connected organization, then
// exercise an organization-scoped read without leaving provider data behind.
package zohobooks_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/e2e"
)

// booksOrganizationsResponse is the provider envelope returned by org list.
type booksOrganizationsResponse struct {
	Code          int                 `json:"code"`
	Organizations []booksOrganization `json:"organizations"`
}

// booksOrganization carries the selector and display name returned by Books.
type booksOrganization struct {
	ID   string `json:"organization_id"`
	Name string `json:"name"`
}

// booksContactsResponse preserves the contacts field even when its array is empty.
type booksContactsResponse struct {
	Code     int             `json:"code"`
	Contacts json.RawMessage `json:"contacts"`
}

// TestE2EOrganizationAndContacts follows the Books org-discovery contract into a real scoped read.
func TestE2EOrganizationAndContacts(t *testing.T) {
	out := mustRunBooks(t, "org", "list", "--json")
	var organizations booksOrganizationsResponse
	decodeBooksJSON(t, out, &organizations)
	if organizations.Code != 0 || len(organizations.Organizations) == 0 {
		t.Fatalf("org list returned no usable organization:\n%s", out)
	}
	org := organizations.Organizations[0]
	if org.ID == "" || org.Name == "" {
		t.Fatalf("org list returned an organization without id or name:\n%s", out)
	}

	// The follow-up call proves the discovered id is accepted on the data plane.
	out = mustRunBooks(t, "contact", "list", "--organization-id", org.ID, "--per-page", "1", "--json")
	var contacts booksContactsResponse
	decodeBooksJSON(t, out, &contacts)
	if contacts.Code != 0 || contacts.Contacts == nil {
		t.Fatalf("contact list returned no contacts envelope for organization %s:\n%s", org.ID, out)
	}
}

// mustRunBooks executes one Books command and requires a successful exit.
func mustRunBooks(t *testing.T, args ...string) string {
	t.Helper()
	out, stderr, exit := e2e.RunToolWithStderr(t, "zoho-books", "", args...)
	if exit != 0 {
		t.Fatalf("%s: exit = %d\nstderr: %s\nstdout: %s", strings.Join(args, " "), exit, stderr, out)
	}
	return out
}

// decodeBooksJSON decodes one provider response into its fixed response type.
func decodeBooksJSON(t *testing.T, out string, dst any) {
	t.Helper()
	if err := json.Unmarshal([]byte(out), dst); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
}
