package calendar

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/spf13/cobra"
)

// eventOptions holds the create/update flag values.
type eventOptions struct {
	calendar    string
	summary     string
	from        string
	to          string
	description string
	location    string
	attendees   []string
	recurrence  []string
	reminders   []int
	allDay      bool
	meet        bool
	sendUpdates string
	timezone    string
}

// requireRFC3339 validates a timed timestamp carries an RFC3339 value with a
// timezone offset (the API rejects naked local times).
func requireRFC3339(name, value string) error {
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return fmt.Errorf("calendar: --%s must be an RFC3339 timestamp with offset (e.g. 2026-07-16T14:00:00-07:00), got %q", name, value)
	}
	return nil
}

// requireDate validates an all-day boundary is a YYYY-MM-DD date.
func requireDate(name, value string) error {
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return fmt.Errorf("calendar: --%s must be YYYY-MM-DD for --all-day, got %q", name, value)
	}
	return nil
}

// eventTimeFor builds a start/end object, validating against --all-day. A
// non-empty timeZone (IANA name) is attached to timed values: Google requires
// an explicit time zone on a recurring event's start/end — a bare numeric
// offset is rejected with "Missing time zone definition for start time".
func eventTimeFor(name, value string, allDay bool, timeZone string) (map[string]any, error) {
	if allDay {
		if err := requireDate(name, value); err != nil {
			return nil, err
		}
		return map[string]any{"date": value}, nil
	}
	if err := requireRFC3339(name, value); err != nil {
		return nil, err
	}
	t := map[string]any{"dateTime": value}
	if timeZone != "" {
		t["timeZone"] = timeZone
	}
	return t, nil
}

// calendarTimeZone fetches a calendar's default IANA time zone from its
// calendarList entry.
func (s *Service) calendarTimeZone(ctx context.Context, token, calendar string) (string, error) {
	if calendar == "" {
		calendar = defaultCalendar
	}
	body, err := s.call(ctx, token, http.MethodGet, "/users/me/calendarList/"+url.PathEscape(calendar), nil, nil)
	if err != nil {
		return "", err
	}
	var entry struct {
		TimeZone string `json:"timeZone"`
	}
	if err := json.Unmarshal(body, &entry); err != nil {
		return "", fmt.Errorf("calendar: decode calendar time zone: %w", err)
	}
	return entry.TimeZone, nil
}

// resolveEventTimeZone decides which IANA time zone to stamp on a timed
// start/end. An explicit --timezone always wins. Otherwise a zone is only
// needed — and only fetched — for a recurring timed event whose times are
// being set: Google rejects those without one. The default matches the
// Calendar UI: the calendar's own time zone. Single (non-recurring) events keep
// working on the offset alone, so no extra lookup happens on the common path.
func (s *Service) resolveEventTimeZone(ctx context.Context, token string, o *eventOptions, timesSet bool) (string, error) {
	if o.timezone != "" {
		return o.timezone, nil
	}
	if len(o.recurrence) == 0 || o.allDay || !timesSet {
		return "", nil
	}
	tz, err := s.calendarTimeZone(ctx, token, o.calendar)
	if err != nil {
		return "", err
	}
	if tz == "" {
		return "", fmt.Errorf("calendar: could not determine the calendar's time zone for a recurring event; pass --timezone (e.g. America/Los_Angeles)")
	}
	return tz, nil
}

// validateSendUpdates guards the sendUpdates enum.
func validateSendUpdates(v string) error {
	switch v {
	case "all", "externalOnly", "none":
		return nil
	}
	return fmt.Errorf("calendar: --send-updates must be all, externalOnly, or none, got %q", v)
}

// buildAttendees maps emails into attendee objects.
func buildAttendees(emails []string) []map[string]any {
	out := make([]map[string]any, 0, len(emails))
	for _, e := range emails {
		out = append(out, map[string]any{"email": e})
	}
	return out
}

// buildReminders maps minute offsets into popup override reminders.
func buildReminders(minutes []int) map[string]any {
	overrides := make([]map[string]any, 0, len(minutes))
	for _, m := range minutes {
		overrides = append(overrides, map[string]any{"method": "popup", "minutes": m})
	}
	return map[string]any{"useDefault": false, "overrides": overrides}
}

// requestID generates a unique conference createRequest id. A fresh id per
// event is mandatory: reusing one can attach an existing conference to the new
// event and expose it to unintended guests (Google's documented warning).
func (s *Service) requestID() string {
	if s.newRequestID != nil {
		return s.newRequestID()
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failure is unrecoverable here; fall back to a
		// time-seeded id so we never reuse the zero value.
		return fmt.Sprintf("helio-%d", time.Now().UnixNano())
	}
	return "helio-" + hex.EncodeToString(b[:])
}

// meetConference builds the conferenceData.createRequest payload for --meet.
func (s *Service) meetConference() map[string]any {
	return map[string]any{
		"createRequest": map[string]any{
			"requestId":             s.requestID(),
			"conferenceSolutionKey": map[string]any{"type": "hangoutsMeet"},
		},
	}
}

func (s *Service) newEventsCreateCmd(token string) *cobra.Command {
	var o eventOptions
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an event (add --attendee to invite; --meet for a Meet link)",
		Args:  cobra.NoArgs,
		// POST /calendars/{cal}/events — mutating provider call (design 318).
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if o.summary == "" {
				return fmt.Errorf("calendar: --summary is required")
			}
			if o.from == "" || o.to == "" {
				return fmt.Errorf("calendar: --from and --to are required")
			}
			if err := validateSendUpdates(o.sendUpdates); err != nil {
				return err
			}
			body := map[string]any{"summary": o.summary}
			tz, err := s.resolveEventTimeZone(cmd.Context(), token, &o, true)
			if err != nil {
				return err
			}
			start, err := eventTimeFor("from", o.from, o.allDay, tz)
			if err != nil {
				return err
			}
			end, err := eventTimeFor("to", o.to, o.allDay, tz)
			if err != nil {
				return err
			}
			body["start"], body["end"] = start, end
			if o.description != "" {
				body["description"] = o.description
			}
			if o.location != "" {
				body["location"] = o.location
			}
			if len(o.attendees) > 0 {
				body["attendees"] = buildAttendees(o.attendees)
			}
			if len(o.recurrence) > 0 {
				body["recurrence"] = o.recurrence
			}
			if len(o.reminders) > 0 {
				body["reminders"] = buildReminders(o.reminders)
			}
			q := url.Values{"sendUpdates": {o.sendUpdates}}
			if o.meet {
				body["conferenceData"] = s.meetConference()
				q.Set("conferenceDataVersion", "1")
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, eventsPath(o.calendar), q, body)
			if err != nil {
				return err
			}
			return s.emitEvent(cmd, resp, "created event")
		},
	}
	addEventWriteFlags(cmd, &o)
	return cmd
}

func (s *Service) newEventsUpdateCmd(token string) *cobra.Command {
	var o eventOptions
	cmd := &cobra.Command{
		Use:   "update <event-id>",
		Short: "Patch an event (only the flags you pass are changed — never a full replace)",
		Args:  cobra.ExactArgs(1),
		// PATCH /calendars/{cal}/events/{id} — mutating provider call (design 318).
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateSendUpdates(o.sendUpdates); err != nil {
				return err
			}
			body := map[string]any{}
			if cmd.Flags().Changed("summary") {
				body["summary"] = o.summary
			}
			timesSet := cmd.Flags().Changed("from") || cmd.Flags().Changed("to")
			tz, err := s.resolveEventTimeZone(cmd.Context(), token, &o, timesSet)
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("from") {
				start, err := eventTimeFor("from", o.from, o.allDay, tz)
				if err != nil {
					return err
				}
				body["start"] = start
			}
			if cmd.Flags().Changed("to") {
				end, err := eventTimeFor("to", o.to, o.allDay, tz)
				if err != nil {
					return err
				}
				body["end"] = end
			}
			if cmd.Flags().Changed("description") {
				body["description"] = o.description
			}
			if cmd.Flags().Changed("location") {
				body["location"] = o.location
			}
			if cmd.Flags().Changed("attendee") {
				body["attendees"] = buildAttendees(o.attendees)
			}
			if cmd.Flags().Changed("recurrence") {
				body["recurrence"] = o.recurrence
			}
			if cmd.Flags().Changed("reminder") {
				body["reminders"] = buildReminders(o.reminders)
			}
			q := url.Values{"sendUpdates": {o.sendUpdates}}
			if o.meet {
				body["conferenceData"] = s.meetConference()
				q.Set("conferenceDataVersion", "1")
			}
			if len(body) == 0 {
				return fmt.Errorf("calendar: nothing to update — pass at least one field flag")
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPatch, eventsPath(o.calendar, args[0]), q, body)
			if err != nil {
				return err
			}
			return s.emitEvent(cmd, resp, "updated event")
		},
	}
	addEventWriteFlags(cmd, &o)
	return cmd
}

func (s *Service) newEventsDeleteCmd(token string) *cobra.Command {
	var calendar, sendUpdates string
	cmd := &cobra.Command{
		Use:   "delete <event-id>",
		Short: "Delete an event (Google keeps it in the trash for 30 days)",
		Args:  cobra.ExactArgs(1),
		// DELETE /calendars/{cal}/events/{id} — mutating provider call (design 318).
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateSendUpdates(sendUpdates); err != nil {
				return err
			}
			q := url.Values{"sendUpdates": {sendUpdates}}
			if _, err := s.call(cmd.Context(), token, http.MethodDelete, eventsPath(calendar, args[0]), q, nil); err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitJSON(map[string]any{"id": args[0], "status": "deleted"})
			}
			fmt.Fprintf(s.stdout(), "deleted event %s\n", args[0])
			return nil
		},
	}
	addCalendarFlag(cmd, &calendar)
	cmd.Flags().StringVar(&sendUpdates, "send-updates", "all", "who to notify: all, externalOnly, or none")
	return cmd
}

func (s *Service) newEventsRespondCmd(token string) *cobra.Command {
	var calendar, status, comment string
	cmd := &cobra.Command{
		Use:   "respond <event-id>",
		Short: "RSVP as yourself (accepted|declined|tentative); the organizer is always notified",
		Args:  cobra.ExactArgs(1),
		// GET then PATCH /calendars/{cal}/events/{id} (read-modify-write of the
		// attendees array) — mutating provider call (design 318).
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			switch status {
			case "accepted", "declined", "tentative":
			default:
				return fmt.Errorf("calendar: --status must be accepted, declined, or tentative, got %q", status)
			}
			// Read-modify-write: the attendees array is replaced wholesale on
			// patch, so we fetch the full list, flip only the self entry, and
			// write every attendee back to avoid dropping the other guests.
			path := eventsPath(calendar, args[0])
			body, err := s.call(cmd.Context(), token, http.MethodGet, path, nil, nil)
			if err != nil {
				return err
			}
			var ev struct {
				Attendees []map[string]any `json:"attendees"`
			}
			if err := json.Unmarshal(body, &ev); err != nil {
				return fmt.Errorf("calendar: decode event attendees: %w", err)
			}
			selfIdx := -1
			for i, a := range ev.Attendees {
				if self, ok := a["self"].(bool); ok && self {
					selfIdx = i
					break
				}
			}
			if selfIdx < 0 {
				return fmt.Errorf("calendar: you are not an attendee of event %s (nothing to respond to)", args[0])
			}
			ev.Attendees[selfIdx]["responseStatus"] = status
			if comment != "" {
				ev.Attendees[selfIdx]["comment"] = comment
			}
			q := url.Values{"sendUpdates": {"all"}}
			resp, err := s.call(cmd.Context(), token, http.MethodPatch, path, q, map[string]any{"attendees": ev.Attendees})
			if err != nil {
				return err
			}
			return s.emitEvent(cmd, resp, "responded "+status+" to event")
		},
	}
	addCalendarFlag(cmd, &calendar)
	cmd.Flags().StringVar(&status, "status", "", "RSVP status: accepted, declined, or tentative (required)")
	cmd.Flags().StringVar(&comment, "comment", "", "optional response comment sent to the organizer")
	return cmd
}

// addEventWriteFlags wires the shared create/update flags.
func addEventWriteFlags(cmd *cobra.Command, o *eventOptions) {
	addCalendarFlag(cmd, &o.calendar)
	cmd.Flags().StringVar(&o.summary, "summary", "", "event title")
	cmd.Flags().StringVar(&o.from, "from", "", "start, RFC3339 (or YYYY-MM-DD with --all-day)")
	cmd.Flags().StringVar(&o.to, "to", "", "end, RFC3339 (or YYYY-MM-DD with --all-day)")
	cmd.Flags().StringVar(&o.description, "description", "", "event description")
	cmd.Flags().StringVar(&o.location, "location", "", "event location")
	cmd.Flags().StringArrayVar(&o.attendees, "attendee", nil, "attendee email (repeatable)")
	cmd.Flags().StringArrayVar(&o.recurrence, "recurrence", nil, "RFC 5545 RRULE line (repeatable, passed through verbatim)")
	cmd.Flags().IntSliceVar(&o.reminders, "reminder", nil, "popup reminder minutes before start (repeatable)")
	cmd.Flags().BoolVar(&o.allDay, "all-day", false, "treat --from/--to as all-day dates (YYYY-MM-DD)")
	cmd.Flags().BoolVar(&o.meet, "meet", false, "attach a Google Meet video link")
	cmd.Flags().StringVar(&o.sendUpdates, "send-updates", "all", "who to notify: all, externalOnly, or none")
	cmd.Flags().StringVar(&o.timezone, "timezone", "", "IANA time zone for timed start/end (e.g. America/Los_Angeles); defaults to the calendar's zone. Recurring timed events require one — Google rejects a bare offset")
}

// emitEvent prints a create/update/respond response.
func (s *Service) emitEvent(cmd *cobra.Command, body []byte, verb string) error {
	if jsonOut(cmd) {
		return s.emit(body)
	}
	var e eventSummary
	if err := json.Unmarshal(body, &e); err != nil {
		return fmt.Errorf("calendar: decode event: %w", err)
	}
	fmt.Fprintf(s.stdout(), "%s %s\n", verb, e.ID)
	renderEventLine(s.stdout(), e)
	if e.HangLink != "" {
		fmt.Fprintf(s.stdout(), "meet: %s\n", e.HangLink)
	}
	return nil
}
