//go:build e2e

// Real-API e2e for Twitch: verify the OAuth-bound user, then resolve that
// identity through the channel endpoint with the required Client-Id header.
package twitch_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/e2e"
)

// twitchUser carries stable identity fields returned by `user get`.
type twitchUser struct {
	ID          string `json:"id"`
	Login       string `json:"login"`
	DisplayName string `json:"display_name"`
}

// twitchChannel carries the broadcaster identity returned by `channel get`.
type twitchChannel struct {
	BroadcasterID    string `json:"broadcaster_id"`
	BroadcasterLogin string `json:"broadcaster_login"`
}

// TestE2EUserAndChannelRead proves both authenticated identity and a second
// Helix resource work with the OAuth token plus configured Client-Id header.
func TestE2EUserAndChannelRead(t *testing.T) {
	out := mustRunTwitch(t, "user", "get", "--json")
	var user twitchUser
	decodeTwitchJSON(t, out, &user)
	if user.ID == "" || user.Login == "" || user.DisplayName == "" {
		t.Fatalf("user get returned no usable account identity:\n%s", out)
	}

	// The follow-up request exercises self-id resolution and confirms the
	// channel response belongs to the same authenticated Twitch account.
	out = mustRunTwitch(t, "channel", "get", "--json")
	var channel twitchChannel
	decodeTwitchJSON(t, out, &channel)
	if channel.BroadcasterID != user.ID || channel.BroadcasterLogin != user.Login {
		t.Fatalf("channel get did not return authenticated broadcaster %s:\n%s", user.ID, out)
	}
}

// mustRunTwitch executes one Twitch command and requires a successful exit.
func mustRunTwitch(t *testing.T, args ...string) string {
	t.Helper()
	out, stderr, exit := e2e.RunToolWithStderr(t, "twitch", "", args...)
	if exit != 0 {
		t.Fatalf("%s: exit = %d\nstderr: %s\nstdout: %s", strings.Join(args, " "), exit, stderr, out)
	}
	return out
}

// decodeTwitchJSON decodes one provider response into its fixed response type.
func decodeTwitchJSON(t *testing.T, out string, dst any) {
	t.Helper()
	if err := json.Unmarshal([]byte(out), dst); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
}
