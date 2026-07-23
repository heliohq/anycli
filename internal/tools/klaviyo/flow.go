package klaviyo

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newFlowCmd builds the `flow` group: list/get plus a status toggle.
func (s *Service) newFlowCmd(token string) *cobra.Command {
	group := newGroupCmd("flow", "Read flows and toggle their status")
	group.AddCommand(
		s.newCollectionListCmd(token, "list", "List flows (GET /flows)", "/flows", "flow"),
		s.newResourceGetCmd(token, "get", "Get one flow (GET /flows/{id})", "/flows/", "flow"),
		s.newFlowStatusCmd(token),
	)
	return group
}

// newFlowStatusCmd builds `flow status` → PATCH /flows/{id} setting the flow's
// status to draft/manual/live.
func (s *Service) newFlowStatusCmd(token string) *cobra.Command {
	var status, data string
	cmd := &cobra.Command{
		Use:         "status <id>",
		Short:       "Set a flow's status (PATCH /flows/{id}) via --status draft|manual|live or --data",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			var payload any
			if data != "" {
				var err error
				if payload, err = parseDataFlag(data); err != nil {
					return err
				}
			} else {
				switch status {
				case "draft", "manual", "live":
				case "":
					return &usageError{msg: "provide --status draft|manual|live, or --data"}
				default:
					return &usageError{msg: "--status must be draft, manual, or live, got " + status}
				}
				payload = resourceBody("flow", args[0], map[string]any{"status": status}, nil)
			}
			body, err := s.call(cmd.Context(), token, http.MethodPatch, "/flows/"+url.PathEscape(args[0]), nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "flow status: draft|manual|live")
	cmd.Flags().StringVar(&data, "data", "", "raw JSON:API request body (overrides --status)")
	return cmd
}
