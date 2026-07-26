package acuity

import (
	"net/http"

	"github.com/spf13/cobra"
)

// longTypeList, longCalendarList, longFormList and longLabelList are the four
// lookup-resource Longs. They sit next to the shared builder because it is the
// builder that fixes the one-verb, no-flag shape all four describe, while the
// ids each one resolves are entirely different.
const (
	longTypeList = "The name-to-id map booking depends on: every command that schedules takes\n" +
		"--type-id and none of them accept a type name. Each entry also carries the\n" +
		"type's duration in minutes and its price, which is what makes a proposed\n" +
		"slot the right length before `availability times` is even consulted."

	longCalendarList = "The name-to-id map for staff members and rooms — --calendar-id everywhere\n" +
		"else in the tool is one of these numeric ids. A calendar carries its own\n" +
		"timezone, and an --admin booking cannot be made without naming one."

	longFormList = "Intake forms and, inside each, the numeric field ids that `--field\n" +
		"<id>=<value>` on `appointment create` and `appointment update` requires; a\n" +
		"field NAME on the left of the `=` is rejected. Each field also reports its\n" +
		"type and whether it is required, which is what a client-mode create is\n" +
		"validated against."

	longLabelList = "Labels are the colored tags an appointment can carry on the calendar. This\n" +
		"is a read-only lookup and nothing in this tool applies or clears one, so a\n" +
		"label can be reported on but not changed from here."
)

// newListOnlyCmd builds a resource group whose single `list` subcommand GETs a
// static collection endpoint and passes the JSON through. Used for the
// read-only lookup resources (appointment types, calendars, forms, labels).
func (s *Service) newListOnlyCmd(token, use, short, long, path string) *cobra.Command {
	group := newGroupCmd(use, short)
	group.AddCommand(&cobra.Command{
		Use:         "list",
		Short:       "List " + use + "s (GET " + path + ")",
		Long:        long,
		Annotations: readOnly,
		Args:        cobra.NoArgs,
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
	return s.newListOnlyCmd(token, "type", "Appointment types (resolve names → ids and durations)", longTypeList, "/appointment-types")
}

func (s *Service) newCalendarCmd(token string) *cobra.Command {
	return s.newListOnlyCmd(token, "calendar", "Calendars (resolve names → ids)", longCalendarList, "/calendars")
}

func (s *Service) newFormCmd(token string) *cobra.Command {
	return s.newListOnlyCmd(token, "form", "Intake forms (field ids needed for booking)", longFormList, "/forms")
}

func (s *Service) newLabelCmd(token string) *cobra.Command {
	return s.newListOnlyCmd(token, "label", "Appointment labels", longLabelList, "/labels")
}

func (s *Service) newMeCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "me",
		Short: "Account identity and settings (GET /me)",
		Long: "The account's own settings: business name, contact email, currency, and the\n" +
			"TIMEZONE that every bare --datetime, --start and --end in this tool is\n" +
			"parsed in. Read it once before writing any time without an explicit UTC\n" +
			"offset, since a mismatch here books the right clock time on the wrong\n" +
			"hour.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/me", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}
