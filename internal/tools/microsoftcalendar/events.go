package microsoftcalendar

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// graphDateTime is Graph's dateTimeTimeZone shape used by event start/end.
type graphDateTime struct {
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone"`
}

// eventSummary is the subset of a Graph event used for human rendering.
type eventSummary struct {
	ID          string        `json:"id"`
	Subject     string        `json:"subject"`
	Start       graphDateTime `json:"start"`
	End         graphDateTime `json:"end"`
	IsCancelled bool          `json:"isCancelled"`
	Location    struct {
		DisplayName string `json:"displayName"`
	} `json:"location"`
	Organizer struct {
		EmailAddress struct {
			Name    string `json:"name"`
			Address string `json:"address"`
		} `json:"emailAddress"`
	} `json:"organizer"`
	OnlineMeeting *struct {
		JoinURL string `json:"joinUrl"`
	} `json:"onlineMeeting"`
	WebLink string `json:"webLink"`
}

func (e eventSummary) window() string {
	start, end := e.Start.DateTime, e.End.DateTime
	if start == "" && end == "" {
		return ""
	}
	return start + " → " + end
}

func (s *Service) newEventsListCmd(token string) *cobra.Command {
	var start, end, filter, page string
	var max int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List events; a --start/--end window queries /me/calendarView, otherwise /me/events",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if (start == "") != (end == "") {
				return fmt.Errorf("microsoft-calendar: --start and --end must be given together for a calendar-view window")
			}
			var path string
			q := url.Values{}
			if page != "" {
				// Graph @odata.nextLink is an absolute URL carrying all
				// original query params; follow it verbatim.
				path = page
			} else {
				if start != "" {
					path = "/me/calendarView"
					q.Set("startDateTime", start)
					q.Set("endDateTime", end)
				} else {
					path = "/me/events"
					q.Set("$orderby", "start/dateTime")
				}
				if filter != "" {
					q.Set("$filter", filter)
				}
				q.Set("$top", strconv.Itoa(max))
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, path, q, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				Value    []eventSummary `json:"value"`
				NextLink string         `json:"@odata.nextLink"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("microsoft-calendar: decode event list: %w", err)
			}
			if len(resp.Value) == 0 {
				fmt.Fprintln(s.stdout(), "no events")
				return nil
			}
			for _, e := range resp.Value {
				subject := e.Subject
				if e.IsCancelled {
					subject = "[cancelled] " + subject
				}
				fmt.Fprintf(s.stdout(), "%s\t%s\t%s\n", e.ID, e.window(), subject)
			}
			if resp.NextLink != "" {
				fmt.Fprintf(s.stdout(), "next page: %s\n", resp.NextLink)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&start, "start", "", "window start (ISO 8601); queries /me/calendarView with --end")
	cmd.Flags().StringVar(&end, "end", "", "window end (ISO 8601); queries /me/calendarView with --start")
	cmd.Flags().StringVar(&filter, "filter", "", "OData $filter passed through verbatim")
	cmd.Flags().IntVar(&max, "max", 10, "max results to return ($top)")
	cmd.Flags().StringVar(&page, "page", "", "@odata.nextLink from a previous list call")
	return cmd
}

func (s *Service) newEventsGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <event-id>",
		Short: "Show one event (GET /me/events/{id})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/me/events/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var e eventSummary
			if err := json.Unmarshal(body, &e); err != nil {
				return fmt.Errorf("microsoft-calendar: decode event: %w", err)
			}
			fmt.Fprintf(s.stdout(), "Subject:  %s\n", e.Subject)
			fmt.Fprintf(s.stdout(), "When:     %s\n", e.window())
			if e.Location.DisplayName != "" {
				fmt.Fprintf(s.stdout(), "Location: %s\n", e.Location.DisplayName)
			}
			if e.Organizer.EmailAddress.Address != "" {
				fmt.Fprintf(s.stdout(), "Organizer: %s\n", e.Organizer.EmailAddress.Address)
			}
			if e.OnlineMeeting != nil && e.OnlineMeeting.JoinURL != "" {
				fmt.Fprintf(s.stdout(), "Join:     %s\n", e.OnlineMeeting.JoinURL)
			}
			if e.IsCancelled {
				fmt.Fprintln(s.stdout(), "Status:   cancelled")
			}
			return nil
		},
	}
}

func (s *Service) newEventsCreateCmd(token string) *cobra.Command {
	var subject, start, end, timezone, body, location string
	var attendees []string
	var online bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an event (POST /me/events)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload := map[string]any{
				"subject": subject,
				"start":   graphDateTime{DateTime: start, TimeZone: timezone},
				"end":     graphDateTime{DateTime: end, TimeZone: timezone},
			}
			if body != "" {
				payload["body"] = map[string]string{"contentType": "text", "content": body}
			}
			if location != "" {
				payload["location"] = map[string]string{"displayName": location}
			}
			if len(attendees) != 0 {
				payload["attendees"] = buildAttendees(attendees)
			}
			if online {
				payload["isOnlineMeeting"] = true
				payload["onlineMeetingProvider"] = "teamsForBusiness"
			}
			respBody, err := s.call(cmd.Context(), token, http.MethodPost, "/me/events", nil, payload)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(respBody)
			}
			var e eventSummary
			if err := json.Unmarshal(respBody, &e); err != nil {
				return fmt.Errorf("microsoft-calendar: decode created event: %w", err)
			}
			fmt.Fprintf(s.stdout(), "created event %s (%s)\n", e.ID, e.window())
			return nil
		},
	}
	cmd.Flags().StringVar(&subject, "subject", "", "event subject")
	cmd.Flags().StringVar(&start, "start", "", "start (ISO 8601, e.g. 2026-07-20T15:00:00)")
	cmd.Flags().StringVar(&end, "end", "", "end (ISO 8601)")
	cmd.Flags().StringVar(&timezone, "timezone", "UTC", "IANA/Windows time zone for --start/--end")
	cmd.Flags().StringArrayVar(&attendees, "attendees", nil, "attendee email (repeatable)")
	cmd.Flags().StringVar(&body, "body", "", "event body text")
	cmd.Flags().StringVar(&location, "location", "", "location display name")
	cmd.Flags().BoolVar(&online, "online", false, "create as an online (Teams) meeting")
	_ = cmd.MarkFlagRequired("subject")
	_ = cmd.MarkFlagRequired("start")
	_ = cmd.MarkFlagRequired("end")
	return cmd
}

func (s *Service) newEventsUpdateCmd(token string) *cobra.Command {
	var start, end, subject, timezone, location string
	cmd := &cobra.Command{
		Use:   "update <event-id>",
		Short: "Update an event's time/subject/location (PATCH /me/events/{id})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{}
			if subject != "" {
				payload["subject"] = subject
			}
			if start != "" {
				payload["start"] = graphDateTime{DateTime: start, TimeZone: timezone}
			}
			if end != "" {
				payload["end"] = graphDateTime{DateTime: end, TimeZone: timezone}
			}
			if location != "" {
				payload["location"] = map[string]string{"displayName": location}
			}
			if len(payload) == 0 {
				return fmt.Errorf("microsoft-calendar: nothing to update — pass --start, --end, --subject, or --location")
			}
			respBody, err := s.call(cmd.Context(), token, http.MethodPatch, "/me/events/"+url.PathEscape(args[0]), nil, payload)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(respBody)
			}
			fmt.Fprintf(s.stdout(), "updated event %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&start, "start", "", "new start (ISO 8601)")
	cmd.Flags().StringVar(&end, "end", "", "new end (ISO 8601)")
	cmd.Flags().StringVar(&subject, "subject", "", "new subject")
	cmd.Flags().StringVar(&timezone, "timezone", "UTC", "IANA/Windows time zone for --start/--end")
	cmd.Flags().StringVar(&location, "location", "", "new location display name")
	return cmd
}

func (s *Service) newEventsCancelCmd(token string) *cobra.Command {
	var comment string
	cmd := &cobra.Command{
		Use:   "cancel <event-id>",
		Short: "Cancel an event and notify attendees (POST /me/events/{id}/cancel)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var payload any
			if comment != "" {
				payload = map[string]string{"comment": comment}
			}
			respBody, err := s.call(cmd.Context(), token, http.MethodPost, "/me/events/"+url.PathEscape(args[0])+"/cancel", nil, payload)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(respBody)
			}
			fmt.Fprintf(s.stdout(), "cancelled event %s (attendees notified)\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&comment, "comment", "", "message included in the cancellation notice")
	return cmd
}

// respondActions maps the user-facing action to Graph's response action verb.
var respondActions = map[string]string{
	"accept":    "accept",
	"decline":   "decline",
	"tentative": "tentativelyAccept",
}

func (s *Service) newEventsRespondCmd(token string) *cobra.Command {
	var action, comment string
	var noNotify bool
	cmd := &cobra.Command{
		Use:   "respond <event-id>",
		Short: "Respond to an invite: accept | decline | tentative",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			verb, ok := respondActions[action]
			if !ok {
				return fmt.Errorf("microsoft-calendar: --action must be accept, decline, or tentative, got %q", action)
			}
			payload := map[string]any{"sendResponse": !noNotify}
			if comment != "" {
				payload["comment"] = comment
			}
			respBody, err := s.call(cmd.Context(), token, http.MethodPost, "/me/events/"+url.PathEscape(args[0])+"/"+verb, nil, payload)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(respBody)
			}
			fmt.Fprintf(s.stdout(), "responded %s to event %s\n", action, args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&action, "action", "", "accept | decline | tentative")
	cmd.Flags().StringVar(&comment, "comment", "", "optional message to the organizer")
	cmd.Flags().BoolVar(&noNotify, "no-notify", false, "do not send a response to the organizer")
	_ = cmd.MarkFlagRequired("action")
	return cmd
}

// buildAttendees maps email addresses to Graph attendee objects.
func buildAttendees(emails []string) []map[string]any {
	out := make([]map[string]any, 0, len(emails))
	for _, e := range emails {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		out = append(out, map[string]any{
			"type":         "required",
			"emailAddress": map[string]string{"address": e},
		})
	}
	return out
}
