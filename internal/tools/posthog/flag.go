package posthog

import (
	"net/url"

	"github.com/spf13/cobra"
)

// newFlagCmd groups feature-flag read and write access. Toggle is a narrow
// PATCH of the `active` field; create/update take a raw JSON body passthrough
// so the full flag schema (filters, rollout, variants) is expressible without
// re-modeling it here.
func (s *Service) newFlagCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "flag", Short: "Feature flags (list, get, create, update, toggle)"}
	cmd.AddCommand(
		s.newProjectListCmd(token, "list", "List feature flags (GET /api/projects/<id>/feature_flags/)", "/feature_flags/", true),
		s.newProjectGetCmd(token, "get", "Get a feature flag (GET /api/projects/<id>/feature_flags/<id>/)", "/feature_flags/"),
		s.newFlagCreateCmd(token),
		s.newFlagUpdateCmd(token),
		s.newFlagToggleCmd(token),
	)
	return cmd
}

func (s *Service) newFlagCreateCmd(token string) *cobra.Command {
	var project, data string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a feature flag (POST /api/projects/<id>/feature_flags/)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireProject(project); err != nil {
				return err
			}
			body, err := rawJSONBody(cmd, "data", data)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, "POST", projectPath(project, "/feature_flags/"), nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project id (required)")
	cmd.Flags().StringVar(&data, "data", "", "flag body as a JSON object file path, or - for stdin (required)")
	return cmd
}

func (s *Service) newFlagUpdateCmd(token string) *cobra.Command {
	var project, id, data string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a feature flag (PATCH /api/projects/<id>/feature_flags/<id>/)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireProject(project); err != nil {
				return err
			}
			if err := requireFlag("id", id); err != nil {
				return err
			}
			body, err := rawJSONBody(cmd, "data", data)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, "PATCH", projectPath(project, "/feature_flags/"+url.PathEscape(id)+"/"), nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project id (required)")
	cmd.Flags().StringVar(&id, "id", "", "feature flag id (required)")
	cmd.Flags().StringVar(&data, "data", "", "partial flag body as a JSON object file path, or - for stdin (required)")
	return cmd
}

// newFlagToggleCmd flips a flag's active state — the single most common flag
// write an agent performs, so it gets a first-class command rather than
// requiring a hand-built --data body.
func (s *Service) newFlagToggleCmd(token string) *cobra.Command {
	var project, id string
	var active bool
	cmd := &cobra.Command{
		Use:   "toggle",
		Short: "Enable or disable a feature flag (PATCH active)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireProject(project); err != nil {
				return err
			}
			if err := requireFlag("id", id); err != nil {
				return err
			}
			if !cmd.Flags().Changed("active") {
				return &usageError{msg: "--active is required (true to enable, false to disable)"}
			}
			resp, err := s.call(cmd.Context(), token, "PATCH", projectPath(project, "/feature_flags/"+url.PathEscape(id)+"/"), nil, map[string]any{"active": active})
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project id (required)")
	cmd.Flags().StringVar(&id, "id", "", "feature flag id (required)")
	cmd.Flags().BoolVar(&active, "active", false, "true to enable, false to disable (required)")
	return cmd
}

// rawJSONBody reads a required JSON-object body from a file/stdin flag.
func rawJSONBody(cmd *cobra.Command, name, path string) (any, error) {
	if path == "" {
		return nil, &usageError{msg: "--" + name + " is required"}
	}
	raw, err := readFileOrStdin(cmd, path)
	if err != nil {
		return nil, &usageError{msg: "read --" + name + ": " + err.Error()}
	}
	return decodeJSONFlag(name, string(raw))
}
