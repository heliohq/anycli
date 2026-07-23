package acuity

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func (s *Service) newAvailabilityCmd(token string) *cobra.Command {
	cmd := newGroupCmd("availability", "Open slots (dates, times)")
	cmd.AddCommand(
		s.newAvailabilityDatesCmd(token),
		s.newAvailabilityTimesCmd(token),
	)
	return cmd
}

func (s *Service) newAvailabilityDatesCmd(token string) *cobra.Command {
	var month, timezone string
	var typeID, calendarID int
	cmd := &cobra.Command{
		Use:   "dates",
		Short: "Days with open slots in a month (GET /availability/dates)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("appointmentTypeID", strconv.Itoa(typeID))
			q.Set("month", month)
			setIntQuery(cmd, q, "calendar-id", "calendarID", calendarID)
			setStringQuery(q, "timezone", timezone)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/availability/dates", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&typeID, "type-id", 0, "appointment type id")
	cmd.Flags().StringVar(&month, "month", "", "month to check (YYYY-MM)")
	cmd.Flags().IntVar(&calendarID, "calendar-id", 0, "limit to a calendar id")
	cmd.Flags().StringVar(&timezone, "timezone", "", "IANA timezone, e.g. America/New_York")
	_ = cmd.MarkFlagRequired("type-id")
	_ = cmd.MarkFlagRequired("month")
	return cmd
}

func (s *Service) newAvailabilityTimesCmd(token string) *cobra.Command {
	var date, timezone string
	var typeID, calendarID int
	cmd := &cobra.Command{
		Use:   "times",
		Short: "Open time slots on a date (GET /availability/times)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("appointmentTypeID", strconv.Itoa(typeID))
			q.Set("date", date)
			setIntQuery(cmd, q, "calendar-id", "calendarID", calendarID)
			setStringQuery(q, "timezone", timezone)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/availability/times", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&typeID, "type-id", 0, "appointment type id")
	cmd.Flags().StringVar(&date, "date", "", "date to check (YYYY-MM-DD)")
	cmd.Flags().IntVar(&calendarID, "calendar-id", 0, "limit to a calendar id")
	cmd.Flags().StringVar(&timezone, "timezone", "", "IANA timezone, e.g. America/New_York")
	_ = cmd.MarkFlagRequired("type-id")
	_ = cmd.MarkFlagRequired("date")
	return cmd
}
