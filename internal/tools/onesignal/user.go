package onesignal

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newUserUpsertCmd(key, appID string) *cobra.Command {
	var aliasLabel, aliasID, properties, tags string
	cmd := &cobra.Command{
		Use:   "upsert",
		Short: "Create or update a user by alias (POST /apps/{app_id}/users)",
		Long: "`--alias-label` and `--alias-id` are both required and together form the\n" +
			"identity — typically `external_id` plus the application's own user id. An\n" +
			"existing user with that alias is updated; otherwise one is created.\n" +
			"`--tags` and `--properties` must each be a JSON OBJECT and are rejected\n" +
			"locally if they are not. Tags are what filter-based segments match on, so\n" +
			"this is how a user becomes eligible for a `segment create --filters`\n" +
			"audience.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if aliasLabel == "" || aliasID == "" {
				return &usageError{msg: "--alias-label and --alias-id are both required"}
			}
			body := map[string]any{
				"identity": map[string]string{aliasLabel: aliasID},
			}
			props := map[string]any{}
			if properties != "" {
				decoded, err := decodeJSONFlag("properties", properties)
				if err != nil {
					return err
				}
				obj, ok := decoded.(map[string]any)
				if !ok {
					return &usageError{msg: "--properties must be a JSON object"}
				}
				props = obj
			}
			if tags != "" {
				decoded, err := decodeJSONFlag("tags", tags)
				if err != nil {
					return err
				}
				if _, ok := decoded.(map[string]any); !ok {
					return &usageError{msg: "--tags must be a JSON object"}
				}
				props["tags"] = decoded
			}
			if len(props) > 0 {
				body["properties"] = props
			}
			resp, err := s.call(cmd.Context(), key, http.MethodPost, appPath(appID, "/users"), nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&aliasLabel, "alias-label", "", "alias label, e.g. external_id (required)")
	cmd.Flags().StringVar(&aliasID, "alias-id", "", "alias value (required)")
	cmd.Flags().StringVar(&properties, "properties", "", "JSON object of user properties (optional)")
	cmd.Flags().StringVar(&tags, "tags", "", "JSON object of user tags (optional)")
	return cmd
}

func (s *Service) newUserGetCmd(key, appID string) *cobra.Command {
	var aliasLabel, aliasID string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Read a user by alias (GET /apps/{app_id}/users/by/{label}/{id})",
		Long: "Users are addressed by an alias PAIR — `--alias-label external_id`\n" +
			"`--alias-id user-123` — and both flags are required; there is no lookup by\n" +
			"a bare OneSignal id, by email or by phone number. A user the app never\n" +
			"stored that alias for is simply not reachable through this command.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if aliasLabel == "" || aliasID == "" {
				return &usageError{msg: "--alias-label and --alias-id are both required"}
			}
			path := appPath(appID, "/users/by/"+url.PathEscape(aliasLabel)+"/"+url.PathEscape(aliasID))
			resp, err := s.call(cmd.Context(), key, http.MethodGet, path, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&aliasLabel, "alias-label", "", "alias label, e.g. external_id (required)")
	cmd.Flags().StringVar(&aliasID, "alias-id", "", "alias value (required)")
	return cmd
}
