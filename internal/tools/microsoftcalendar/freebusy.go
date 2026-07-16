package microsoftcalendar

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"

	"github.com/spf13/cobra"
)

// busyWindow is one merged busy interval in the freebusy result.
type busyWindow struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// freeShowAs marks the Graph showAs states that leave a slot available; any
// other state (busy / tentative / oof / workingElsewhere) counts as busy.
var freeShowAs = map[string]bool{"free": true, "unknown": true}

// newFreebusyCmd computes the signed-in user's OWN busy windows from
// /me/calendarView (covered by Calendars.ReadWrite). Reading OTHER attendees'
// free/busy (findMeetingTimes / getSchedule) needs Calendars.Read.Shared and is
// out of v1 (design 308 §microsoft_calendar), so it is intentionally not here.
func (s *Service) newFreebusyCmd(token string) *cobra.Command {
	var start, end string
	cmd := &cobra.Command{
		Use:   "freebusy",
		Short: "Compute your own busy windows in a time range (from /me/calendarView)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("startDateTime", start)
			q.Set("endDateTime", end)
			q.Set("$orderby", "start/dateTime")
			q.Set("$top", strconv.Itoa(200))
			// Page through the whole window: a range with more than one $top
			// page of events must not silently drop the overflow, or the
			// computed availability would report busy slots as free.
			var raw []busyWindow
			path := "/me/calendarView"
			query := q
			for {
				body, err := s.call(cmd.Context(), token, http.MethodGet, path, query, nil)
				if err != nil {
					return err
				}
				var resp struct {
					Value []struct {
						ShowAs      string        `json:"showAs"`
						IsCancelled bool          `json:"isCancelled"`
						Start       graphDateTime `json:"start"`
						End         graphDateTime `json:"end"`
					} `json:"value"`
					NextLink string `json:"@odata.nextLink"`
				}
				if err := json.Unmarshal(body, &resp); err != nil {
					return fmt.Errorf("microsoft-calendar: decode calendar view: %w", err)
				}
				for _, e := range resp.Value {
					if e.IsCancelled || freeShowAs[e.ShowAs] {
						continue
					}
					if e.Start.DateTime == "" || e.End.DateTime == "" {
						continue
					}
					raw = append(raw, busyWindow{Start: e.Start.DateTime, End: e.End.DateTime})
				}
				if resp.NextLink == "" {
					break
				}
				// @odata.nextLink is an absolute URL carrying all paging state.
				path = resp.NextLink
				query = nil
			}
			busy := mergeBusy(raw)
			if jsonOut(cmd) {
				return s.emitJSON(map[string]any{"start": start, "end": end, "busy": busy})
			}
			if len(busy) == 0 {
				fmt.Fprintln(s.stdout(), "no busy windows — fully free")
				return nil
			}
			for _, w := range busy {
				fmt.Fprintf(s.stdout(), "busy\t%s → %s\n", w.Start, w.End)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&start, "start", "", "window start (ISO 8601)")
	cmd.Flags().StringVar(&end, "end", "", "window end (ISO 8601)")
	_ = cmd.MarkFlagRequired("start")
	_ = cmd.MarkFlagRequired("end")
	return cmd
}

// mergeBusy sorts and coalesces overlapping/adjacent busy intervals. Graph
// returns calendarView times in a single consistent zone (UTC by default), so
// same-format ISO 8601 strings order and compare lexicographically.
func mergeBusy(in []busyWindow) []busyWindow {
	if len(in) == 0 {
		return nil
	}
	sort.Slice(in, func(i, j int) bool {
		if in[i].Start != in[j].Start {
			return in[i].Start < in[j].Start
		}
		return in[i].End < in[j].End
	})
	out := []busyWindow{in[0]}
	for _, w := range in[1:] {
		last := &out[len(out)-1]
		if w.Start <= last.End {
			if w.End > last.End {
				last.End = w.End
			}
			continue
		}
		out = append(out, w)
	}
	return out
}
