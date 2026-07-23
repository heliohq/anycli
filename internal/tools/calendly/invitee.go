package calendly

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// newInviteeNoShowCmd wraps the invitee no-show writes. Without --undo it marks
// an invitee as a no-show (POST /invitee_no_shows, body {invitee: <invitee
// URI>}); with --undo it clears a no-show (DELETE /invitee_no_shows/{uuid}). The
// argument is the full invitee URI when marking (a bare UUID is ambiguous
// because an invitee URI is nested under its event), and the no-show URI/UUID
// when undoing.
func (s *Service) newInviteeNoShowCmd(token string) *cobra.Command {
	var undo bool
	cmd := &cobra.Command{
		Use:         "no-show <invitee-uri|no-show-id>",
		Short:       "Mark an invitee no-show, or clear one with --undo",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			if undo {
				resp, err := s.call(cmd.Context(), token, http.MethodDelete, "/invitee_no_shows/"+url.PathEscape(uuidOf(args[0])), nil, nil)
				if err != nil {
					return err
				}
				return s.emit(resp)
			}
			if !strings.Contains(args[0], "://") {
				return &usageError{msg: "calendly: invitee no-show requires the full invitee URI (…/scheduled_events/{uuid}/invitees/{uuid}); a bare UUID is ambiguous"}
			}
			body := map[string]any{"invitee": args[0]}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/invitee_no_shows", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().BoolVar(&undo, "undo", false, "clear a no-show instead of creating one (arg is the no-show id/URI)")
	return cmd
}
