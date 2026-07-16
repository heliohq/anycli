package microsoftcalendar

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

func (s *Service) newCalendarsListCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the signed-in user's calendars (GET /me/calendars)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/me/calendars", nil, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				Value []struct {
					ID        string `json:"id"`
					Name      string `json:"name"`
					IsDefault bool   `json:"isDefaultCalendar"`
					CanEdit   bool   `json:"canEdit"`
					Owner     struct {
						Name    string `json:"name"`
						Address string `json:"address"`
					} `json:"owner"`
				} `json:"value"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("microsoft-calendar: decode calendar list: %w", err)
			}
			if len(resp.Value) == 0 {
				fmt.Fprintln(s.stdout(), "no calendars")
				return nil
			}
			for _, c := range resp.Value {
				marker := ""
				if c.IsDefault {
					marker = " (default)"
				}
				fmt.Fprintf(s.stdout(), "%s\t%s%s\towner=%s\n", c.ID, c.Name, marker, c.Owner.Address)
			}
			return nil
		},
	}
}
