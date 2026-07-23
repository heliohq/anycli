package acuity

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func (s *Service) newAppointmentCmd(token string) *cobra.Command {
	cmd := newGroupCmd("appointment", "Appointments (list, get, book, edit, reschedule, cancel)")
	cmd.AddCommand(
		s.newAppointmentListCmd(token),
		s.newAppointmentGetCmd(token),
		s.newAppointmentCreateCmd(token),
		s.newAppointmentUpdateCmd(token),
		s.newAppointmentRescheduleCmd(token),
		s.newAppointmentCancelCmd(token),
	)
	return cmd
}

func (s *Service) newAppointmentListCmd(token string) *cobra.Command {
	var minDate, maxDate, email, firstName, lastName, direction string
	var calendarID, typeID, max int
	var canceled, excludeForms bool
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List scheduled appointments (GET /appointments)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setStringQuery(q, "minDate", minDate)
			setStringQuery(q, "maxDate", maxDate)
			setStringQuery(q, "email", email)
			setStringQuery(q, "firstName", firstName)
			setStringQuery(q, "lastName", lastName)
			setStringQuery(q, "direction", direction)
			setIntQuery(cmd, q, "calendar-id", "calendarID", calendarID)
			setIntQuery(cmd, q, "type-id", "appointmentTypeID", typeID)
			setIntQuery(cmd, q, "max", "max", max)
			if canceled {
				q.Set("canceled", "true")
			}
			if excludeForms {
				q.Set("excludeForms", "true")
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/appointments", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&minDate, "min-date", "", "appointments on/after this date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&maxDate, "max-date", "", "appointments on/before this date (YYYY-MM-DD)")
	cmd.Flags().IntVar(&calendarID, "calendar-id", 0, "filter by calendar id")
	cmd.Flags().IntVar(&typeID, "type-id", 0, "filter by appointment type id")
	cmd.Flags().StringVar(&email, "email", "", "filter by client email")
	cmd.Flags().StringVar(&firstName, "first-name", "", "filter by client first name")
	cmd.Flags().StringVar(&lastName, "last-name", "", "filter by client last name")
	cmd.Flags().BoolVar(&canceled, "canceled", false, "return canceled appointments instead of active ones")
	cmd.Flags().BoolVar(&excludeForms, "exclude-forms", false, "omit intake forms to speed up the response")
	cmd.Flags().IntVar(&max, "max", 0, "maximum number of results (Acuity default 100)")
	cmd.Flags().StringVar(&direction, "direction", "", "sort order: ASC or DESC (Acuity default DESC)")
	return cmd
}

func (s *Service) newAppointmentGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <id>",
		Short:       "Get one appointment (GET /appointments/:id)",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/appointments/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newAppointmentCreateCmd(token string) *cobra.Command {
	var datetime, firstName, lastName, email, phone, timezone, notes string
	var typeID, calendarID int
	var fieldArgs []string
	var admin, noEmail bool
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Book an appointment (POST /appointments)",
		Annotations: writeAction,
		Long: "Book an appointment. --datetime is passed through verbatim (Acuity parses it " +
			"via strtotime in the business/calendar timezone); ISO-8601 (2026-07-15T09:00:00-0400) " +
			"is the safe form. --admin bypasses availability checks and requires --calendar-id.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fields, err := parseFields(fieldArgs)
			if err != nil {
				return err
			}
			body := map[string]any{
				"appointmentTypeID": typeID,
				"datetime":          datetime,
				"firstName":         firstName,
				"lastName":          lastName,
			}
			setStringIfSet(body, "email", email)
			setStringIfSet(body, "phone", phone)
			setStringIfSet(body, "timezone", timezone)
			setStringIfSet(body, "notes", notes)
			setIntIfChanged(cmd, body, "calendar-id", "calendarID", calendarID)
			if len(fields) > 0 {
				body["fields"] = fields
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/appointments", adminEmailQuery(admin, noEmail), body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&typeID, "type-id", 0, "appointment type id")
	cmd.Flags().StringVar(&datetime, "datetime", "", "appointment start (ISO-8601 recommended)")
	cmd.Flags().StringVar(&firstName, "first-name", "", "client first name")
	cmd.Flags().StringVar(&lastName, "last-name", "", "client last name")
	cmd.Flags().StringVar(&email, "email", "", "client email (optional for admin bookings)")
	cmd.Flags().StringVar(&phone, "phone", "", "client phone")
	cmd.Flags().StringVar(&timezone, "timezone", "", "IANA timezone, e.g. America/New_York")
	cmd.Flags().IntVar(&calendarID, "calendar-id", 0, "calendar id (required with --admin)")
	cmd.Flags().StringVar(&notes, "notes", "", "appointment notes (admin bookings only)")
	cmd.Flags().StringArrayVar(&fieldArgs, "field", nil, "intake form answer id=value (repeatable)")
	cmd.Flags().BoolVar(&admin, "admin", false, "book as admin: bypass availability/attribute validation")
	cmd.Flags().BoolVar(&noEmail, "no-email", false, "suppress confirmation email/SMS")
	_ = cmd.MarkFlagRequired("type-id")
	_ = cmd.MarkFlagRequired("datetime")
	_ = cmd.MarkFlagRequired("first-name")
	_ = cmd.MarkFlagRequired("last-name")
	return cmd
}

func (s *Service) newAppointmentUpdateCmd(token string) *cobra.Command {
	var firstName, lastName, email, phone, notes string
	var fieldArgs []string
	var admin, noEmail bool
	cmd := &cobra.Command{
		Use:         "update <id>",
		Short:       "Edit an appointment's client details / intake fields (PUT /appointments/:id)",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fields, err := parseFields(fieldArgs)
			if err != nil {
				return err
			}
			body := map[string]any{}
			setStringIfSet(body, "firstName", firstName)
			setStringIfSet(body, "lastName", lastName)
			setStringIfSet(body, "email", email)
			setStringIfSet(body, "phone", phone)
			setStringIfSet(body, "notes", notes)
			if len(fields) > 0 {
				body["fields"] = fields
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPut, "/appointments/"+url.PathEscape(args[0]), adminEmailQuery(admin, noEmail), body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&firstName, "first-name", "", "client first name")
	cmd.Flags().StringVar(&lastName, "last-name", "", "client last name")
	cmd.Flags().StringVar(&email, "email", "", "client email")
	cmd.Flags().StringVar(&phone, "phone", "", "client phone")
	cmd.Flags().StringVar(&notes, "notes", "", "appointment notes")
	cmd.Flags().StringArrayVar(&fieldArgs, "field", nil, "intake form answer id=value (repeatable)")
	cmd.Flags().BoolVar(&admin, "admin", false, "edit as admin")
	cmd.Flags().BoolVar(&noEmail, "no-email", false, "suppress notification email/SMS")
	return cmd
}

func (s *Service) newAppointmentRescheduleCmd(token string) *cobra.Command {
	var datetime, timezone string
	var calendarID int
	var admin, noEmail bool
	cmd := &cobra.Command{
		Use:         "reschedule <id>",
		Short:       "Move an appointment to a new time (PUT /appointments/:id/reschedule)",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{"datetime": datetime}
			setStringIfSet(body, "timezone", timezone)
			setIntIfChanged(cmd, body, "calendar-id", "calendarID", calendarID)
			resp, err := s.call(cmd.Context(), token, http.MethodPut, "/appointments/"+url.PathEscape(args[0])+"/reschedule", adminEmailQuery(admin, noEmail), body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&datetime, "datetime", "", "new appointment start (ISO-8601 recommended)")
	cmd.Flags().StringVar(&timezone, "timezone", "", "client timezone")
	cmd.Flags().IntVar(&calendarID, "calendar-id", 0, "move to this calendar id (default: keep current)")
	cmd.Flags().BoolVar(&admin, "admin", false, "reschedule as admin: disable availability validation")
	cmd.Flags().BoolVar(&noEmail, "no-email", false, "suppress reschedule email/SMS")
	_ = cmd.MarkFlagRequired("datetime")
	return cmd
}

func (s *Service) newAppointmentCancelCmd(token string) *cobra.Command {
	var note string
	var admin, noEmail bool
	cmd := &cobra.Command{
		Use:         "cancel <id>",
		Short:       "Cancel an appointment (PUT /appointments/:id/cancel)",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{}
			setStringIfSet(body, "cancelNote", note)
			resp, err := s.call(cmd.Context(), token, http.MethodPut, "/appointments/"+url.PathEscape(args[0])+"/cancel", adminEmailQuery(admin, noEmail), body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&note, "note", "", "message sent with the cancellation notification (cancelNote)")
	cmd.Flags().BoolVar(&admin, "admin", false, "cancel as admin: disable cancellation rules")
	cmd.Flags().BoolVar(&noEmail, "no-email", false, "suppress cancellation email/SMS")
	return cmd
}

// setStringQuery sets a query param only when the value is non-empty.
func setStringQuery(q url.Values, key, value string) {
	if value != "" {
		q.Set(key, value)
	}
}

// setIntQuery sets a query param from an int flag only when the user changed it.
func setIntQuery(cmd *cobra.Command, q url.Values, flag, key string, value int) {
	if cmd.Flags().Changed(flag) {
		q.Set(key, strconv.Itoa(value))
	}
}
