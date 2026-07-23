package fullstory

import (
	"net/http"

	"github.com/spf13/cobra"
)

func (s *Service) newEventCmd(key string) *cobra.Command {
	cmd := &cobra.Command{Use: "event", Short: "Server-side custom events"}
	cmd.AddCommand(s.newEventCreateCmd(key))
	return cmd
}

// newEventCreateCmd wraps POST /v2/events — record one server-side custom event.
// FullStory requires exactly ONE identification form: by user (--uid) or by
// explicit session (--session-id); supplying both is a 400, so the tool rejects
// it up front as a usage error. --use-recent is a modifier on the user path
// (stitch the event into the user's most recent session) and therefore needs
// --uid and is incompatible with --session-id.
func (s *Service) newEventCreateCmd(key string) *cobra.Command {
	var name, uid, sessionID, timestamp string
	var useRecent bool
	var props []string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Record a custom event for a user or session (POST /v2/events)",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if name == "" {
				return &usageError{msg: "event create requires --name"}
			}
			if uid == "" && sessionID == "" {
				return &usageError{msg: "event create requires --uid or --session-id"}
			}
			if uid != "" && sessionID != "" {
				return &usageError{msg: "event create accepts only one of --uid or --session-id"}
			}
			if useRecent && uid == "" {
				return &usageError{msg: "event create --use-recent requires --uid"}
			}
			properties, perr := parseProps(props)
			if perr != nil {
				return perr
			}
			body := map[string]any{"name": name}
			if timestamp != "" {
				body["timestamp"] = timestamp
			}
			if properties != nil {
				body["properties"] = properties
			}
			if sessionID != "" {
				body["session"] = map[string]any{"id": sessionID}
			} else {
				body["user"] = map[string]any{"uid": uid}
				if useRecent {
					body["session"] = map[string]any{"use_most_recent": true}
				}
			}
			resp, err := s.call(cmd.Context(), key, http.MethodPost, "/v2/events", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "custom event name (required)")
	cmd.Flags().StringVar(&uid, "uid", "", "application-specific user id")
	cmd.Flags().StringVar(&sessionID, "session-id", "", "FullStory session id (deviceId:sessionId)")
	cmd.Flags().BoolVar(&useRecent, "use-recent", false, "stitch into the user's most recent session (needs --uid)")
	cmd.Flags().StringVar(&timestamp, "timestamp", "", "ISO-8601 event timestamp (optional)")
	cmd.Flags().StringArrayVar(&props, "prop", nil, "custom property key=value (repeatable)")
	return cmd
}
