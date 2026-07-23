package salesloft

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newNoteCmd groups qualitative logging: list notes and add a note to a person
// or account.
func (s *Service) newNoteCmd(token string) *cobra.Command {
	cmd := newGroupCmd("note", "Manage notes")
	cmd.AddCommand(
		s.newNoteListCmd(token),
		s.newNoteCreateCmd(token),
	)
	return cmd
}

func (s *Service) newNoteListCmd(token string) *cobra.Command {
	var lf listFlags
	var assocType string
	var assocIDs []string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List notes (GET /v2/notes); filter by --associated-with-type / --associated-with-id",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q, err := lf.values()
			if err != nil {
				return err
			}
			if assocType != "" {
				q.Set("associated_with_type", assocType)
			}
			for _, id := range assocIDs {
				q.Add("associated_with_id[]", id)
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/notes", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerListFlags(cmd, &lf)
	cmd.Flags().StringVar(&assocType, "associated-with-type", "", "association type: person|account")
	cmd.Flags().StringArrayVar(&assocIDs, "associated-with-id", nil, "association record id (repeatable)")
	return cmd
}

func (s *Service) newNoteCreateCmd(token string) *cobra.Command {
	var content, assocType string
	var assocID int
	var skipActivities bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a note on a person or account (POST /v2/notes)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{
				"content":              content,
				"associated_with_type": assocType,
				"associated_with_id":   assocID,
			}
			if cmd.Flags().Changed("skip-activities") {
				body["skip_activities"] = skipActivities
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/notes", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&content, "content", "", "note content")
	cmd.Flags().StringVar(&assocType, "associated-with-type", "", "association type: person|account")
	cmd.Flags().IntVar(&assocID, "associated-with-id", 0, "association record id")
	cmd.Flags().BoolVar(&skipActivities, "skip-activities", false, "do not create an activity for this note")
	_ = cmd.MarkFlagRequired("content")
	_ = cmd.MarkFlagRequired("associated-with-type")
	_ = cmd.MarkFlagRequired("associated-with-id")
	return cmd
}
