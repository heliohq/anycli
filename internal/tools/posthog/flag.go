package posthog

import (
	"net/url"

	"github.com/spf13/cobra"
)

// longFlagList and longFlagGet live here because both leaves are built by the
// shared project-scoped constructors, which take their prose as a parameter.
const (
	longFlagList = "`--search` matches the flag key and name. Each row carries the flag's\n" +
		"`key`, its `active` state and the `filters` object holding rollout groups\n" +
		"and variants. The numeric `id` is what `flag get`, `flag update` and\n" +
		"`flag toggle` take — not the string key application code references."

	longFlagGet = "Takes the numeric flag id from `flag list`, not the string key. Worth the\n" +
		"extra call before any write: `flag toggle` shows no preview of what it is\n" +
		"about to change, and both writes return only the resulting state."
)

// newFlagCmd groups feature-flag read and write access. Toggle is a narrow
// PATCH of the `active` field; create/update take a raw JSON body passthrough
// so the full flag schema (filters, rollout, variants) is expressible without
// re-modeling it here.
func (s *Service) newFlagCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "flag", Short: "Feature flags (list, get, create, update, toggle)"}
	cmd.AddCommand(
		s.newProjectListCmd(token, "list", "List feature flags (GET /api/projects/<id>/feature_flags/)", longFlagList, "/feature_flags/", true),
		s.newProjectGetCmd(token, "get", "Get a feature flag (GET /api/projects/<id>/feature_flags/<id>/)", longFlagGet, "/feature_flags/"),
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
		Long: "--data names a JSON file, or - for stdin, holding the whole flag body:\n" +
			"`key`, `name`, `active`, and the `filters` object that carries release\n" +
			"groups, rollout percentages and multivariate variants. The body is passed\n" +
			"through unmodeled, so it is PostHog's own validation that rejects a bad\n" +
			"one. A flag created with `active` true is live for real users the moment\n" +
			"this returns.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
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
		Long: "PATCH semantics: --data holds only the fields to change and anything\n" +
			"omitted keeps its current value. `filters` is one field, though — to edit\n" +
			"part of a rollout, send the whole object as `flag get` returned it with the\n" +
			"change applied, not the fragment. --id is the numeric flag id, not the key.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
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
		Long: "Patches the `active` field alone and takes effect for real users\n" +
			"IMMEDIATELY — there is no staged or scheduled state, and no undo beyond\n" +
			"toggling back. --active is required and has no implicit default; omitting\n" +
			"it is a usage error rather than a disable. Rollout `filters` are left\n" +
			"untouched, so a flag re-enabled here resumes at whatever percentage and\n" +
			"variant split it was left at.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
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
