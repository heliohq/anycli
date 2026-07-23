package acuity

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newBlockCmd(token string) *cobra.Command {
	cmd := newGroupCmd("block", "Blocked-off time (list, create, delete)")
	cmd.AddCommand(
		s.newBlockListCmd(token),
		s.newBlockCreateCmd(token),
		s.newBlockDeleteCmd(token),
	)
	return cmd
}

func (s *Service) newBlockListCmd(token string) *cobra.Command {
	var minDate, maxDate string
	var calendarID, max int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List blocked-off time (GET /blocks)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setStringQuery(q, "minDate", minDate)
			setStringQuery(q, "maxDate", maxDate)
			setIntQuery(cmd, q, "calendar-id", "calendarID", calendarID)
			setIntQuery(cmd, q, "max", "max", max)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/blocks", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&minDate, "min-date", "", "blocks on/after this date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&maxDate, "max-date", "", "blocks on/before this date (YYYY-MM-DD)")
	cmd.Flags().IntVar(&calendarID, "calendar-id", 0, "filter by calendar id")
	cmd.Flags().IntVar(&max, "max", 0, "maximum number of results")
	return cmd
}

func (s *Service) newBlockCreateCmd(token string) *cobra.Command {
	var start, end, notes string
	var calendarID int
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Block off time (POST /blocks)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{"start": start, "end": end}
			setStringIfSet(body, "notes", notes)
			setIntIfChanged(cmd, body, "calendar-id", "calendarID", calendarID)
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/blocks", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&start, "start", "", "block start (ISO-8601 recommended)")
	cmd.Flags().StringVar(&end, "end", "", "block end (ISO-8601 recommended)")
	cmd.Flags().IntVar(&calendarID, "calendar-id", 0, "calendar id to block")
	cmd.Flags().StringVar(&notes, "notes", "", "block notes")
	_ = cmd.MarkFlagRequired("start")
	_ = cmd.MarkFlagRequired("end")
	return cmd
}

func (s *Service) newBlockDeleteCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a block (DELETE /blocks/:id)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodDelete, "/blocks/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}
