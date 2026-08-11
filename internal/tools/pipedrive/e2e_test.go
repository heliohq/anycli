//go:build e2e

// Real-API e2e for Pipedrive: verify the authenticated user and exercise an
// activity lifecycle that cleans up every record it creates.
package pipedrive_test

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/e2e"
)

// pipedriveUserResponse is the provider envelope returned by user me.
type pipedriveUserResponse struct {
	Success bool          `json:"success"`
	Data    pipedriveUser `json:"data"`
}

// pipedriveUser carries the stable identity fields checked by the smoke test.
type pipedriveUser struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
}

// pipedriveActivityResponse is the provider envelope returned by activity
// create, get, and update commands.
type pipedriveActivityResponse struct {
	Success bool              `json:"success"`
	Data    pipedriveActivity `json:"data"`
}

// pipedriveActivity carries the stable fields used to verify lifecycle state.
type pipedriveActivity struct {
	ID        int64  `json:"id"`
	Subject   string `json:"subject"`
	IsDeleted bool   `json:"is_deleted"`
}

// TestE2EUserMe proves both the OAuth access token and its per-company
// api_domain credential can reach the authenticated Pipedrive account.
func TestE2EUserMe(t *testing.T) {
	out, stderr, exit := e2e.RunToolWithStderr(t, "pipedrive", "", "--json", "user", "me")
	if exit != 0 {
		t.Fatalf("user me: exit = %d\nstderr: %s\nstdout: %s", exit, stderr, out)
	}

	// Decode the fixed provider envelope before checking the identity fields.
	var response pipedriveUserResponse
	if err := json.Unmarshal([]byte(out), &response); err != nil {
		t.Fatalf("user me output is not valid JSON: %v\n%s", err, out)
	}
	if !response.Success || response.Data.ID == 0 || strings.TrimSpace(response.Data.Email) == "" {
		t.Fatalf("user me returned an incomplete identity:\n%s", out)
	}
}

// TestE2EActivityClosedLoop catches broken activity write verbs, response
// decoding, and cleanup by exercising create, get, update, delete, then get.
func TestE2EActivityClosedLoop(t *testing.T) {
	subject := e2e.Prefix() + "pipedrive-activity"
	updatedSubject := subject + "-updated"
	activityID := int64(0)
	deleted := false

	// Remove a created activity when a later assertion stops the test early.
	defer func() {
		if activityID == 0 || deleted {
			return
		}
		_, stderr, exit := e2e.RunToolWithStderr(t, "pipedrive", "", "--json", "activity", "delete", formatID(activityID))
		if exit != 0 {
			t.Errorf("cleanup activity %d: exit = %d\nstderr: %s", activityID, exit, stderr)
		}
	}()

	// Create an isolated task activity whose run-scoped subject identifies it.
	out, stderr, exit := e2e.RunToolWithStderr(t, "pipedrive", "", "--json", "activity", "create", "--subject", subject, "--type", "task")
	if exit != 0 {
		t.Fatalf("activity create: exit = %d\nstderr: %s\nstdout: %s", exit, stderr, out)
	}
	created := decodePipedriveActivity(t, "activity create", out)
	if created.ID == 0 || created.Subject != subject {
		t.Fatalf("activity create returned unexpected data: %+v", created)
	}
	activityID = created.ID

	// Read the activity back to verify creation was durably visible.
	out, stderr, exit = e2e.RunToolWithStderr(t, "pipedrive", "", "--json", "activity", "get", formatID(activityID))
	if exit != 0 {
		t.Fatalf("activity get after create: exit = %d\nstderr: %s\nstdout: %s", exit, stderr, out)
	}
	if got := decodePipedriveActivity(t, "activity get after create", out); got.ID != activityID || got.Subject != subject {
		t.Fatalf("activity get after create returned unexpected data: %+v", got)
	}

	// Update the subject and confirm the write is visible through a fresh read.
	out, stderr, exit = e2e.RunToolWithStderr(t, "pipedrive", "", "--json", "activity", "update", formatID(activityID), "--subject", updatedSubject)
	if exit != 0 {
		t.Fatalf("activity update: exit = %d\nstderr: %s\nstdout: %s", exit, stderr, out)
	}
	updated := decodePipedriveActivity(t, "activity update", out)
	if updated.ID != activityID || updated.Subject != updatedSubject {
		t.Fatalf("activity update returned unexpected data: %+v", updated)
	}

	// Delete the activity and mark cleanup complete before probing absence.
	out, stderr, exit = e2e.RunToolWithStderr(t, "pipedrive", "", "--json", "activity", "delete", formatID(activityID))
	if exit != 0 {
		t.Fatalf("activity delete: exit = %d\nstderr: %s\nstdout: %s", exit, stderr, out)
	}
	deleted = true

	// Pipedrive retains a tombstone, so verify the follow-up read marks it deleted.
	out, stderr, exit = e2e.RunToolWithStderr(t, "pipedrive", "", "--json", "activity", "get", formatID(activityID))
	if exit != 0 {
		t.Fatalf("activity get after delete: exit = %d\nstderr: %s\nstdout: %s", exit, stderr, out)
	}
	deletedActivity := decodePipedriveActivity(t, "activity get after delete", out)
	if deletedActivity.ID != activityID || !deletedActivity.IsDeleted {
		t.Fatalf("activity get after delete returned a live record: %+v", deletedActivity)
	}
}

// decodePipedriveActivity parses one fixed activity response and fails the
// calling test when the provider envelope is incomplete.
func decodePipedriveActivity(t *testing.T, step, out string) pipedriveActivity {
	t.Helper()
	var response pipedriveActivityResponse
	if err := json.Unmarshal([]byte(out), &response); err != nil {
		t.Fatalf("%s output is not valid JSON: %v\n%s", step, err, out)
	}
	if !response.Success || response.Data.ID == 0 {
		t.Fatalf("%s returned an incomplete activity:\n%s", step, out)
	}
	return response.Data
}

// formatID formats a Pipedrive numeric identifier for a command argument.
func formatID(id int64) string {
	return strconv.FormatInt(id, 10)
}
