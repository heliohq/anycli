package close

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newActivityCmd is the activity group. Reads cover every activity type
// (note/call/email/sms/meeting/…) via the shared list/get/delete verbs; writes
// ship a `note-add` convenience plus a generic `create <type> --data` escape
// hatch that posts a raw JSON body to /activity/<type>/ (call, email, etc.).
func (s *Service) newActivityCmd(token string) *cobra.Command {
	group := newGroupCmd("activity", "Read and log activities on a lead")
	group.AddCommand(
		s.newActivityListCmd(token),
		s.newActivityGetCmd(token),
		s.newActivityDeleteCmd(token),
		s.newActivityNoteAddCmd(token),
		s.newActivityCreateCmd(token),
	)
	return group
}

func (s *Service) newActivityListCmd(token string) *cobra.Command {
	var (
		lf       listFlags
		leadID   string
		typeName string
	)
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List activities, optionally filtered by lead and type",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			lf.apply(q)
			if leadID != "" {
				q.Set("lead_id", leadID)
			}
			if typeName != "" {
				q.Set("_type", typeName)
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/activity/", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	registerListFlags(cmd, &lf)
	cmd.Flags().StringVar(&leadID, "lead-id", "", "restrict to one lead (Close lead_id)")
	cmd.Flags().StringVar(&typeName, "type", "", "restrict to one activity type (Close _type, e.g. Note, Call, Email)")
	return cmd
}

func (s *Service) newActivityGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <type> <id>",
		Short:       "Get one activity by type and id (type ∈ note|call|email|sms|meeting)",
		Args:        cobra.ExactArgs(2),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, activityPath(args[0], args[1]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newActivityDeleteCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "delete <type> <id>",
		Short:       "Delete one activity by type and id",
		Args:        cobra.ExactArgs(2),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodDelete, activityPath(args[0], args[1]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newActivityNoteAddCmd(token string) *cobra.Command {
	var (
		leadID string
		note   string
	)
	cmd := &cobra.Command{
		Use:         "note-add --lead-id <id> --note <text>",
		Short:       "Add a note activity to a lead",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/activity/note/", nil, map[string]any{
				"lead_id": leadID,
				"note":    note,
			})
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&leadID, "lead-id", "", "lead the note belongs to")
	cmd.Flags().StringVar(&note, "note", "", "note text")
	_ = cmd.MarkFlagRequired("lead-id")
	_ = cmd.MarkFlagRequired("note")
	return cmd
}

// newActivityCreateCmd is the generic activity writer: POST /activity/<type>/
// with a raw JSON body, covering call/email/sms/meeting and any custom fields
// that the typed convenience verbs do not expose.
func (s *Service) newActivityCreateCmd(token string) *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:         "create <type> --data <json|@file>",
		Short:       "Create an activity of a given type from a JSON body (call, email, sms, meeting, …)",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := readData("data", data)
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/activity/"+url.PathEscape(args[0])+"/", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "JSON object (or @file.json) for the new activity")
	return cmd
}

// activityPath builds /activity/<type>/<id>/ with both segments escaped.
func activityPath(activityType, id string) string {
	return "/activity/" + url.PathEscape(activityType) + "/" + url.PathEscape(id) + "/"
}
