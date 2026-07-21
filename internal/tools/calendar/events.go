package calendar

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// eventTime mirrors the Calendar API start/end object: exactly one of dateTime
// (timed) or date (all-day) is set.
type eventTime struct {
	DateTime string `json:"dateTime,omitempty"`
	Date     string `json:"date,omitempty"`
	TimeZone string `json:"timeZone,omitempty"`
}

// when returns the human-readable instant, preferring the timed value.
func (t eventTime) when() string {
	if t.DateTime != "" {
		return t.DateTime
	}
	if t.Date != "" {
		return t.Date + " (all-day)"
	}
	return ""
}

// eventSummary is the subset of an Events resource the human list/get summary
// renders.
type eventSummary struct {
	ID       string    `json:"id"`
	Summary  string    `json:"summary"`
	Status   string    `json:"status"`
	Start    eventTime `json:"start"`
	End      eventTime `json:"end"`
	HangLink string    `json:"hangoutLink"`
}

// eventsPath builds the /calendars/{cal}/events[/suffix] path with both
// segments escaped.
func eventsPath(calendar string, suffix ...string) string {
	p := "/calendars/" + url.PathEscape(calendar) + "/events"
	for _, s := range suffix {
		p += "/" + url.PathEscape(s)
	}
	return p
}

func (s *Service) newEventsListCmd(token string) *cobra.Command {
	var calendar, query, from, to, pageToken, orderBy string
	var max int
	var singleEvents bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List events (native full-text --query; RFC3339 --from/--to time window)",
		Args:  cobra.NoArgs,
		// GET /calendars/{cal}/events — read-only (design 318).
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if orderBy != "" && orderBy != "startTime" && orderBy != "updated" {
				return fmt.Errorf("calendar: --order-by must be startTime or updated, got %q", orderBy)
			}
			if orderBy == "startTime" && !singleEvents {
				return fmt.Errorf("calendar: --order-by startTime requires --single-events")
			}
			q := url.Values{}
			if query != "" {
				q.Set("q", query)
			}
			if from != "" {
				if err := requireRFC3339("from", from); err != nil {
					return err
				}
				q.Set("timeMin", from)
			}
			if to != "" {
				if err := requireRFC3339("to", to); err != nil {
					return err
				}
				q.Set("timeMax", to)
			}
			if singleEvents {
				q.Set("singleEvents", "true")
			}
			if orderBy != "" {
				q.Set("orderBy", orderBy)
			}
			q.Set("maxResults", strconv.Itoa(max))
			if pageToken != "" {
				q.Set("pageToken", pageToken)
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, eventsPath(calendar), q, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				Items         []eventSummary `json:"items"`
				NextPageToken string         `json:"nextPageToken"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("calendar: decode event list: %w", err)
			}
			if len(resp.Items) == 0 {
				fmt.Fprintln(s.stdout(), "no events")
				return nil
			}
			for _, e := range resp.Items {
				renderEventLine(s.stdout(), e)
			}
			if resp.NextPageToken != "" {
				fmt.Fprintf(s.stdout(), "next page token: %s\n", resp.NextPageToken)
			}
			return nil
		},
	}
	addCalendarFlag(cmd, &calendar)
	cmd.Flags().StringVar(&query, "query", "", "full-text search (passed through as the API q param)")
	cmd.Flags().StringVar(&from, "from", "", "lower time bound, RFC3339 (timeMin)")
	cmd.Flags().StringVar(&to, "to", "", "upper time bound, RFC3339 (timeMax)")
	cmd.Flags().IntVar(&max, "max", 10, "max results to return")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "page token from a previous list call")
	cmd.Flags().BoolVar(&singleEvents, "single-events", false, "expand recurring events into instances")
	cmd.Flags().StringVar(&orderBy, "order-by", "", "sort order: startTime (needs --single-events) or updated")
	return cmd
}

func (s *Service) newEventsGetCmd(token string) *cobra.Command {
	var calendar string
	cmd := &cobra.Command{
		Use:   "get <event-id>",
		Short: "Show one event",
		Args:  cobra.ExactArgs(1),
		// GET /calendars/{cal}/events/{id} — read-only (design 318).
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, eventsPath(calendar, args[0]), nil, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var e eventSummary
			if err := json.Unmarshal(body, &e); err != nil {
				return fmt.Errorf("calendar: decode event: %w", err)
			}
			renderEventLine(s.stdout(), e)
			return nil
		},
	}
	addCalendarFlag(cmd, &calendar)
	return cmd
}

func (s *Service) newEventsInstancesCmd(token string) *cobra.Command {
	var calendar, from, to, pageToken string
	var max int
	cmd := &cobra.Command{
		Use:   "instances <event-id>",
		Short: "List the concrete instances of a recurring event (query one before editing a single occurrence)",
		Args:  cobra.ExactArgs(1),
		// GET /calendars/{cal}/events/{id}/instances — read-only (design 318).
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			if from != "" {
				if err := requireRFC3339("from", from); err != nil {
					return err
				}
				q.Set("timeMin", from)
			}
			if to != "" {
				if err := requireRFC3339("to", to); err != nil {
					return err
				}
				q.Set("timeMax", to)
			}
			q.Set("maxResults", strconv.Itoa(max))
			if pageToken != "" {
				q.Set("pageToken", pageToken)
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, eventsPath(calendar, args[0], "instances"), q, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				Items         []eventSummary `json:"items"`
				NextPageToken string         `json:"nextPageToken"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("calendar: decode instances: %w", err)
			}
			if len(resp.Items) == 0 {
				fmt.Fprintln(s.stdout(), "no instances")
				return nil
			}
			for _, e := range resp.Items {
				renderEventLine(s.stdout(), e)
			}
			if resp.NextPageToken != "" {
				fmt.Fprintf(s.stdout(), "next page token: %s\n", resp.NextPageToken)
			}
			return nil
		},
	}
	addCalendarFlag(cmd, &calendar)
	cmd.Flags().StringVar(&from, "from", "", "lower time bound, RFC3339 (timeMin)")
	cmd.Flags().StringVar(&to, "to", "", "upper time bound, RFC3339 (timeMax)")
	cmd.Flags().IntVar(&max, "max", 25, "max results to return")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "page token from a previous call")
	return cmd
}

// addCalendarFlag wires the shared --calendar flag defaulting to primary.
func addCalendarFlag(cmd *cobra.Command, calendar *string) {
	cmd.Flags().StringVar(calendar, "calendar", defaultCalendar, "calendar id (default primary)")
}

// renderEventLine prints one event's id, title, start, and an optional status /
// Meet marker.
func renderEventLine(w io.Writer, e eventSummary) {
	title := e.Summary
	if title == "" {
		title = "(no title)"
	}
	line := fmt.Sprintf("%s\t%s\t%s", e.ID, e.Start.when(), title)
	if e.Status == "cancelled" {
		line += "\t[cancelled]"
	}
	if e.HangLink != "" {
		line += "\tmeet=" + e.HangLink
	}
	fmt.Fprintln(w, line)
}
