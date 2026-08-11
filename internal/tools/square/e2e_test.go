//go:build e2e

// Real-API e2e for Square: discover the connected seller's locations, then
// retrieve one location through the same sandbox or production endpoint.
package square_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/e2e"
)

// squareLocationsResponse is the provider envelope returned by location list.
type squareLocationsResponse struct {
	Locations []squareLocation `json:"locations"`
}

// squareLocationResponse is the provider envelope returned by location get.
type squareLocationResponse struct {
	Location squareLocation `json:"location"`
}

// squareLocation carries the stable seller identity checked across both calls.
type squareLocation struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// squareCustomerResponse is the provider envelope returned by customer
// create, get, and update commands.
type squareCustomerResponse struct {
	Customer squareCustomer `json:"customer"`
}

// squareCustomer carries the stable fields checked across the write chain.
type squareCustomer struct {
	ID        string `json:"id"`
	GivenName string `json:"given_name"`
	Note      string `json:"note"`
}

// squareCustomerRequest is the fixed create/update body sent by the test.
type squareCustomerRequest struct {
	GivenName string `json:"given_name,omitempty"`
	Note      string `json:"note,omitempty"`
}

// TestE2ELocationRead follows location discovery into a real location read
// without creating payments, orders, customers, or other provider data.
func TestE2ELocationRead(t *testing.T) {
	out := mustRunSquare(t, "location", "list", "--json")
	var locations squareLocationsResponse
	decodeSquareJSON(t, out, &locations)
	if len(locations.Locations) == 0 {
		t.Fatalf("location list returned no seller locations:\n%s", out)
	}
	location := locations.Locations[0]
	if location.ID == "" || location.Name == "" {
		t.Fatalf("location list returned a location without id or name:\n%s", out)
	}

	// The follow-up call proves the discovered id and credential-scoped base
	// URL are accepted together by the connected Square environment.
	out = mustRunSquare(t, "location", "get", "--location-id", location.ID, "--json")
	var fetched squareLocationResponse
	decodeSquareJSON(t, out, &fetched)
	if fetched.Location.ID != location.ID || fetched.Location.Name != location.Name {
		t.Fatalf("location get did not return discovered location %s with name %q:\n%s", location.ID, location.Name, out)
	}
}

// TestE2ECustomerClosedLoop catches broken customer write verbs and cleanup by
// exercising create, get, update, delete, then get-after-delete.
func TestE2ECustomerClosedLoop(t *testing.T) {
	name := e2e.Prefix() + "square-customer"
	updatedNote := e2e.Prefix() + "updated"
	customerID := ""
	deleted := false

	// Remove a created customer when a later assertion stops the test early.
	defer func() {
		if customerID == "" || deleted {
			return
		}
		_, stderr, exit := e2e.RunToolWithStderr(t, "square", "", "customer", "delete", "--customer-id", customerID, "--json")
		if exit != 0 {
			t.Errorf("cleanup customer %s: exit = %d\nstderr: %s", customerID, exit, stderr)
		}
	}()

	// Create an isolated customer whose run-scoped name identifies it.
	out := mustRunSquare(t, "customer", "create", "--body", encodeSquareJSON(t, squareCustomerRequest{GivenName: name}), "--json")
	created := decodeSquareCustomer(t, "customer create", out)
	if created.ID == "" || created.GivenName != name {
		t.Fatalf("customer create returned unexpected data: %+v", created)
	}
	customerID = created.ID

	// Read the customer back to verify creation was durably visible.
	out = mustRunSquare(t, "customer", "get", "--customer-id", customerID, "--json")
	if got := decodeSquareCustomer(t, "customer get after create", out); got.ID != customerID || got.GivenName != name {
		t.Fatalf("customer get after create returned unexpected data: %+v", got)
	}

	// Update the note and confirm the mutation through a fresh read.
	mustRunSquare(t, "customer", "update", "--customer-id", customerID, "--body", encodeSquareJSON(t, squareCustomerRequest{Note: updatedNote}), "--json")
	out = mustRunSquare(t, "customer", "get", "--customer-id", customerID, "--json")
	if got := decodeSquareCustomer(t, "customer get after update", out); got.ID != customerID || got.Note != updatedNote {
		t.Fatalf("customer get after update returned unexpected data: %+v", got)
	}

	// Delete the customer and mark the deferred cleanup complete.
	mustRunSquare(t, "customer", "delete", "--customer-id", customerID, "--json")
	deleted = true

	// A deleted Square customer must no longer be retrievable.
	out, stderr, exit := e2e.RunToolWithStderr(t, "square", "", "customer", "get", "--customer-id", customerID, "--json")
	if exit == 0 || !strings.Contains(stderr, `"status":404`) {
		t.Fatalf("customer get after delete: exit = %d, want API 404\nstderr: %s\nstdout: %s", exit, stderr, out)
	}
}

// mustRunSquare executes one Square command and requires a successful exit.
func mustRunSquare(t *testing.T, args ...string) string {
	t.Helper()
	out, stderr, exit := e2e.RunToolWithStderr(t, "square", "", args...)
	if exit != 0 {
		t.Fatalf("%s: exit = %d\nstderr: %s\nstdout: %s", strings.Join(args, " "), exit, stderr, out)
	}
	return out
}

// decodeSquareJSON decodes one provider response into its fixed response type.
func decodeSquareJSON(t *testing.T, out string, dst any) {
	t.Helper()
	if err := json.Unmarshal([]byte(out), dst); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
}

// decodeSquareCustomer parses one fixed customer response and requires an id.
func decodeSquareCustomer(t *testing.T, step, out string) squareCustomer {
	t.Helper()
	var response squareCustomerResponse
	decodeSquareJSON(t, out, &response)
	if response.Customer.ID == "" {
		t.Fatalf("%s returned a customer without an id:\n%s", step, out)
	}
	return response.Customer
}

// encodeSquareJSON serializes the fixed request bodies used by the write test.
func encodeSquareJSON(t *testing.T, value squareCustomerRequest) string {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode Square request: %v", err)
	}
	return string(body)
}
