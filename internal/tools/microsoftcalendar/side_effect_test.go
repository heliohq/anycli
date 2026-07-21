package microsoftcalendar

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestSideEffectAnnotations asserts every runnable leaf command of the tree
// carries an explicit anycli.side_effect annotation with the reviewed value
// (design 318 may-mutate criterion), and that group commands carry none.
func TestSideEffectAnnotations(t *testing.T) {
	want := map[string]string{
		"microsoft-calendar calendars list": "false", // GET /me/calendars
		"microsoft-calendar events list":    "false", // GET /me/events or /me/calendarView
		"microsoft-calendar events get":     "false", // GET /me/events/{id}
		"microsoft-calendar events create":  "true",  // POST /me/events
		"microsoft-calendar events update":  "true",  // PATCH /me/events/{id}
		"microsoft-calendar events cancel":  "true",  // POST /me/events/{id}/cancel
		"microsoft-calendar events respond": "true",  // POST /me/events/{id}/{accept|decline|tentativelyAccept}
		"microsoft-calendar freebusy":       "false", // GET /me/calendarView + local merge
	}

	root := (&Service{}).NewCommandTree()
	got := map[string]string{}
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		val, ok := cmd.Annotations["anycli.side_effect"]
		if cmd.HasSubCommands() {
			if ok {
				t.Errorf("%s: group command must not carry anycli.side_effect, got %q", cmd.CommandPath(), val)
			}
			for _, sub := range cmd.Commands() {
				walk(sub)
			}
			return
		}
		if cmd.RunE == nil && cmd.Run == nil {
			return
		}
		if !ok {
			t.Errorf("%s: runnable leaf missing explicit anycli.side_effect annotation", cmd.CommandPath())
			return
		}
		got[cmd.CommandPath()] = val
	}
	walk(root)

	for path, wantVal := range want {
		if gotVal, ok := got[path]; !ok {
			t.Errorf("%s: leaf command not found in tree", path)
		} else if gotVal != wantVal {
			t.Errorf("%s: anycli.side_effect = %q, want %q", path, gotVal, wantVal)
		}
	}
	for path := range got {
		if _, ok := want[path]; !ok {
			t.Errorf("%s: new runnable leaf not covered by this table — classify it per the design 318 may-mutate criterion", path)
		}
	}
}
