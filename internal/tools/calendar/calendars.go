package calendar

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// calendarListEntry is the subset of a calendarList resource the human summary
// renders. calendarList is the user-facing view: the only path to a
// calendarId, its time zone, and the caller's access role.
type calendarListEntry struct {
	ID         string `json:"id"`
	Summary    string `json:"summary"`
	TimeZone   string `json:"timeZone"`
	AccessRole string `json:"accessRole"`
	Primary    bool   `json:"primary"`
}

func (s *Service) newCalendarsListCmd(token string) *cobra.Command {
	var pageToken string
	var max int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the calendars the user is subscribed to (calendarList.list)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("maxResults", strconv.Itoa(max))
			if pageToken != "" {
				q.Set("pageToken", pageToken)
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/users/me/calendarList", q, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				Items         []calendarListEntry `json:"items"`
				NextPageToken string              `json:"nextPageToken"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("calendar: decode calendar list: %w", err)
			}
			if len(resp.Items) == 0 {
				fmt.Fprintln(s.stdout(), "no calendars")
				return nil
			}
			for _, c := range resp.Items {
				renderCalendarLine(s.stdout(), c)
			}
			if resp.NextPageToken != "" {
				fmt.Fprintf(s.stdout(), "next page token: %s\n", resp.NextPageToken)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&max, "max", 100, "max results to return")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "page token from a previous list call")
	return cmd
}

func (s *Service) newCalendarsGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <calendar-id>",
		Short: "Show one calendar list entry (calendarList.get)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/users/me/calendarList/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var c calendarListEntry
			if err := json.Unmarshal(body, &c); err != nil {
				return fmt.Errorf("calendar: decode calendar: %w", err)
			}
			renderCalendarLine(s.stdout(), c)
			return nil
		},
	}
}

// renderCalendarLine prints one calendar's id, name, access role, time zone,
// and a primary marker.
func renderCalendarLine(w interface{ Write([]byte) (int, error) }, c calendarListEntry) {
	primary := ""
	if c.Primary {
		primary = " (primary)"
	}
	fmt.Fprintf(w, "%s\t%s\taccess=%s\ttz=%s%s\n", c.ID, c.Summary, c.AccessRole, c.TimeZone, primary)
}
