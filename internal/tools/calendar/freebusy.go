package calendar

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
)

// maxFreebusyCalendars is the Calendar API cap on calendars per freeBusy query
// (calendarExpansionMax max value; the API returns tooManyCalendarsRequested
// beyond it). Enforced client-side for a clearer message than the raw API error.
const maxFreebusyCalendars = 50

func (s *Service) newFreebusyCmd(token string) *cobra.Command {
	var calendars []string
	var from, to string
	cmd := &cobra.Command{
		Use:   "freebusy",
		Short: "Query busy intervals for calendars (the right way to find a free slot — never events list on someone else's calendar)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ids := splitCalendarIDs(calendars)
			if len(ids) == 0 {
				return fmt.Errorf("calendar: --calendar is required (one or more calendar ids)")
			}
			if len(ids) > maxFreebusyCalendars {
				return fmt.Errorf("calendar: too many calendars (%d) — the Calendar API accepts at most %d per freebusy query; split into batches", len(ids), maxFreebusyCalendars)
			}
			if from == "" || to == "" {
				return fmt.Errorf("calendar: --from and --to are required")
			}
			if err := requireRFC3339("from", from); err != nil {
				return err
			}
			if err := requireRFC3339("to", to); err != nil {
				return err
			}
			items := make([]map[string]any, 0, len(ids))
			for _, id := range ids {
				items = append(items, map[string]any{"id": id})
			}
			payload := map[string]any{"timeMin": from, "timeMax": to, "items": items}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/freeBusy", nil, payload)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			return s.renderFreebusy(body)
		},
	}
	cmd.Flags().StringArrayVar(&calendars, "calendar", nil, "calendar id (repeatable or comma-separated)")
	cmd.Flags().StringVar(&from, "from", "", "start of the window, RFC3339 (timeMin)")
	cmd.Flags().StringVar(&to, "to", "", "end of the window, RFC3339 (timeMax)")
	return cmd
}

// splitCalendarIDs flattens repeated and comma-separated --calendar values,
// dropping blanks.
func splitCalendarIDs(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			if p := strings.TrimSpace(part); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

// renderFreebusy prints per-calendar busy intervals and any inline errors (a
// calendar the caller cannot see busy info for returns an errors entry, not a
// top-level failure).
func (s *Service) renderFreebusy(body []byte) error {
	var resp struct {
		Calendars map[string]struct {
			Busy []struct {
				Start string `json:"start"`
				End   string `json:"end"`
			} `json:"busy"`
			Errors []struct {
				Domain string `json:"domain"`
				Reason string `json:"reason"`
			} `json:"errors"`
		} `json:"calendars"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("calendar: decode freebusy: %w", err)
	}
	if len(resp.Calendars) == 0 {
		fmt.Fprintln(s.stdout(), "no calendars in response")
		return nil
	}
	for id, cal := range resp.Calendars {
		fmt.Fprintf(s.stdout(), "%s:\n", id)
		for _, e := range cal.Errors {
			fmt.Fprintf(s.stdout(), "  error: %s (%s) — the calendar owner may need to share free/busy\n", e.Reason, e.Domain)
		}
		if len(cal.Busy) == 0 && len(cal.Errors) == 0 {
			fmt.Fprintln(s.stdout(), "  free (no busy intervals)")
			continue
		}
		for _, b := range cal.Busy {
			fmt.Fprintf(s.stdout(), "  busy: %s → %s\n", b.Start, b.End)
		}
	}
	return nil
}
