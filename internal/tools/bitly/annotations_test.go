package bitly

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestSideEffectAnnotations pins the anycli.side_effect fact on every runnable
// leaf of the command tree (design 318 may-mutate criterion) and asserts group
// commands carry no annotation. Notable calls: `link expand` is a POST-shaped
// documented lookup (never mutates) → false; `qr image` may write a local file
// but issues only GET → false.
func TestSideEffectAnnotations(t *testing.T) {
	want := map[string]string{
		// link
		"link shorten": "true",  // POST /shorten
		"link create":  "true",  // POST /bitlinks
		"link expand":  "false", // POST /expand — documented lookup
		"link get":     "false",
		"link update":  "true", // PATCH /bitlinks/{bitlink}
		"link list":    "false",
		// analytics (all GET)
		"analytics clicks":               "false",
		"analytics clicks-summary":       "false",
		"analytics countries":            "false",
		"analytics cities":               "false",
		"analytics devices":              "false",
		"analytics referrers":            "false",
		"analytics referrer-name":        "false",
		"analytics referrers-by-domains": "false",
		"analytics referring-domains":    "false",
		"analytics engagements":          "false",
		"analytics engagements-summary":  "false",
		// group (all GET)
		"group list":           "false",
		"group get":            "false",
		"group tags":           "false",
		"group shorten-counts": "false",
		"group clicks":         "false",
		"group countries":      "false",
		"group referrers":      "false",
		"group devices":        "false",
		"group cities":         "false",
		// qr
		"qr create":        "true", // POST /qr-codes
		"qr create-static": "true", // POST /qr-codes/static
		"qr get":           "false",
		"qr list":          "false",
		"qr update":        "true",  // PATCH /qr-codes/{qrcode_id}
		"qr image":         "false", // GET; --output is a local write
		// qr scans (all GET)
		"qr scans scans":     "false",
		"qr scans summary":   "false",
		"qr scans countries": "false",
		"qr scans cities":    "false",
		"qr scans device-os": "false",
		"qr scans browsers":  "false",
		// campaign / channel
		"campaign list":           "false",
		"campaign get":            "false",
		"campaign create":         "true", // POST /campaigns
		"campaign update":         "true", // PATCH /campaigns/{campaign_guid}
		"campaign channel list":   "false",
		"campaign channel get":    "false",
		"campaign channel create": "true", // POST /channels
		"campaign channel update": "true", // PATCH /channels/{channel_guid}
		// user
		"user get": "false", // GET /user
	}

	seen := map[string]string{}
	var walk func(cmd *cobra.Command, path []string)
	walk = func(cmd *cobra.Command, path []string) {
		if cmd.HasSubCommands() {
			if _, ok := cmd.Annotations["anycli.side_effect"]; ok {
				t.Errorf("group command %q must not carry anycli.side_effect", strings.Join(path, " "))
			}
			for _, sub := range cmd.Commands() {
				walk(sub, append(path, sub.Name()))
			}
			return
		}
		if cmd.RunE == nil && cmd.Run == nil {
			return
		}
		key := strings.Join(path, " ")
		got, ok := cmd.Annotations["anycli.side_effect"]
		if !ok {
			t.Errorf("leaf %q missing explicit anycli.side_effect annotation", key)
			return
		}
		seen[key] = got
	}
	root := (&Service{}).NewCommandTree()
	walk(root, nil)

	for key, wantVal := range want {
		got, ok := seen[key]
		if !ok {
			t.Errorf("expected leaf %q not found in command tree", key)
			continue
		}
		if got != wantVal {
			t.Errorf("leaf %q: anycli.side_effect = %q, want %q", key, got, wantVal)
		}
	}
	for key := range seen {
		if _, ok := want[key]; !ok {
			t.Errorf("leaf %q present in tree but missing from expectation table — classify it", key)
		}
	}
}
