//go:build e2e

// Real-API e2e for Hootsuite: verify the OAuth-bound member, then follow
// social-profile discovery into a concrete read when the account has one.
package hootsuite_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/e2e"
)

// hootsuiteMember carries the stable identity returned by me.
type hootsuiteMember struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	FullName string `json:"fullName"`
}

// hootsuiteProfile carries the stable social-profile fields shared by list and
// get responses.
type hootsuiteProfile struct {
	ID                    string `json:"id"`
	Type                  string `json:"type"`
	SocialNetworkUsername string `json:"socialNetworkUsername"`
}

// TestE2EMemberRead proves the OAuth token resolves a real member without
// requiring the connected account to belong to a Hootsuite organization.
func TestE2EMemberRead(t *testing.T) {
	out := mustRunHootsuite(t, "me", "--json")
	var member hootsuiteMember
	decodeHootsuiteJSON(t, out, &member)
	if member.ID == "" || (strings.TrimSpace(member.Email) == "" && strings.TrimSpace(member.FullName) == "") {
		t.Fatalf("me returned no usable member identity:\n%s", out)
	}
}

// TestE2EProfileRead verifies collection decoding and, when available, follows
// a discovered social-profile id through the resource endpoint.
func TestE2EProfileRead(t *testing.T) {
	out := mustRunHootsuite(t, "profile", "list", "--json")
	var profiles []hootsuiteProfile
	decodeHootsuiteJSON(t, out, &profiles)
	if profiles == nil {
		t.Fatalf("profile list returned a null collection:\n%s", out)
	}
	if len(profiles) == 0 {
		return
	}
	discovered := profiles[0]
	if discovered.ID == "" || discovered.Type == "" {
		t.Fatalf("profile list returned a profile without identity:\n%s", out)
	}

	// The follow-up request proves a real id is accepted by the dedicated
	// resource path with the same OAuth credential.
	out = mustRunHootsuite(t, "profile", "get", discovered.ID, "--json")
	var fetched hootsuiteProfile
	decodeHootsuiteJSON(t, out, &fetched)
	if fetched.ID != discovered.ID || fetched.Type != discovered.Type {
		t.Fatalf("profile get did not return discovered profile %s:\n%s", discovered.ID, out)
	}
}

// mustRunHootsuite executes one Hootsuite command and requires a successful exit.
func mustRunHootsuite(t *testing.T, args ...string) string {
	t.Helper()
	out, stderr, exit := e2e.RunToolWithStderr(t, "hootsuite", "", args...)
	if exit != 0 {
		t.Fatalf("%s: exit = %d\nstderr: %s\nstdout: %s", strings.Join(args, " "), exit, stderr, out)
	}
	return out
}

// decodeHootsuiteJSON decodes one provider response into its fixed response type.
func decodeHootsuiteJSON(t *testing.T, out string, dst any) {
	t.Helper()
	if err := json.Unmarshal([]byte(out), dst); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
}
