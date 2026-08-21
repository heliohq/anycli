//go:build e2e

// Real-API e2e for Supabase: verify the OAuth token can list projects without
// creating, updating, querying, or deleting provider resources.
package supabase_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/e2e"
)

// supabaseProject carries the stable identity fields returned by projects list.
type supabaseProject struct {
	ID   string `json:"id"`
	Ref  string `json:"ref"`
	Name string `json:"name"`
}

// TestE2EProjectsRead proves the real Management API accepts the injected
// credential and returns the official CLI's JSON project collection.
func TestE2EProjectsRead(t *testing.T) {
	out, stderr, exit := e2e.RunToolWithStderr(t, "supabase", "", "projects", "list")
	if exit != 0 {
		t.Fatalf("projects list exit = %d\nstderr: %s\nstdout: %s", exit, stderr, out)
	}

	var projects []supabaseProject
	if err := json.Unmarshal([]byte(out), &projects); err != nil {
		t.Fatalf("projects list output is not a JSON collection: %v\n%s", err, out)
	}
	if projects == nil {
		t.Fatalf("projects list returned a null collection:\n%s", out)
	}
	for i, project := range projects {
		if project.ID == "" || project.Ref == "" || strings.TrimSpace(project.Name) == "" {
			t.Fatalf("projects list item %d has no usable identity:\n%s", i, out)
		}
	}
}
