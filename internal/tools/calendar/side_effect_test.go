package calendar

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestSideEffectAnnotations asserts every runnable leaf command of the tree
// carries an explicit anycli.side_effect annotation with the reviewed value
// (design 318 may-mutate criterion), and that group commands carry none.
func TestSideEffectAnnotations(t *testing.T) {
	want := map[string]string{
		"calendar calendars list":   "false", // GET /users/me/calendarList
		"calendar calendars get":    "false", // GET /users/me/calendarList/{id}
		"calendar events list":      "false", // GET /calendars/{cal}/events
		"calendar events get":       "false", // GET /calendars/{cal}/events/{id}
		"calendar events instances": "false", // GET /calendars/{cal}/events/{id}/instances
		"calendar events create":    "true",  // POST /calendars/{cal}/events
		"calendar events update":    "true",  // PATCH /calendars/{cal}/events/{id}
		"calendar events delete":    "true",  // DELETE /calendars/{cal}/events/{id}
		"calendar events respond":   "true",  // GET + PATCH read-modify-write of attendees
		"calendar freebusy":         "false", // POST /freeBusy — documented pure query, cannot mutate
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
