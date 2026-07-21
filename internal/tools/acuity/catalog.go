package acuity

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newListOnlyCmd builds a resource group whose single `list` subcommand GETs a
// static collection endpoint and passes the JSON through. Used for the
// read-only lookup resources (appointment types, calendars, forms, labels).
func (s *Service) newListOnlyCmd(token, use, short, path string) *cobra.Command {
	group := newGroupCmd(use, short)
	group.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List " + use + "s (GET " + path + ")",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, path, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	})
	return group
}

func (s *Service) newTypeCmd(token string) *cobra.Command {
	return s.newListOnlyCmd(token, "type", "Appointment types (resolve names → ids and durations)", "/appointment-types")
}

func (s *Service) newCalendarCmd(token string) *cobra.Command {
	return s.newListOnlyCmd(token, "calendar", "Calendars (resolve names → ids)", "/calendars")
}

func (s *Service) newFormCmd(token string) *cobra.Command {
	return s.newListOnlyCmd(token, "form", "Intake forms (field ids needed for booking)", "/forms")
}

func (s *Service) newLabelCmd(token string) *cobra.Command {
	return s.newListOnlyCmd(token, "label", "Appointment labels", "/labels")
}

func (s *Service) newMeCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "me",
		Short: "Account identity and settings (GET /me)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/me", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}
