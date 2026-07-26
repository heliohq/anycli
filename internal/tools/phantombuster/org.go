package phantombuster

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newOrgGetCmd fetches the current workspace/org identity.
// GET /orgs/fetch → raw object (id is a string).
func (s *Service) newOrgGetCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "Get the current workspace/org identity (GET /orgs/fetch)",
		Long: "Workspace identity, including the org-level `s3Folder` that prefixes the\n" +
			"public URL of every Phantom's result files. Quota and remaining execution\n" +
			"time are NOT here — that is `org resources`.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			raw, err := s.call(cmd.Context(), key, http.MethodGet, "/orgs/fetch", nil, nil)
			if err != nil {
				return err
			}
			return s.emitObject(raw, nil)
		},
	}
}

// newOrgResourcesCmd fetches the org's resources and usage/quota.
// GET /orgs/fetch-resources. Check this before launching — a launch over quota
// fails 429 mid-run with no recoverable partial result.
func (s *Service) newOrgResourcesCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:   "resources",
		Short: "Get org resources, usage, and remaining quota (GET /orgs/fetch-resources)",
		Long: "The pre-launch check. Reports the plan's execution-time budget and how much\n" +
			"of it remains, alongside slot and storage usage. Running out does not\n" +
			"queue or throttle anything: the container is killed with a 429 and its\n" +
			"work is unrecoverable, so one call here can save an entire run.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			raw, err := s.call(cmd.Context(), key, http.MethodGet, "/orgs/fetch-resources", nil, nil)
			if err != nil {
				return err
			}
			return s.emitObject(raw, nil)
		},
	}
}

// newMeCmd fetches the current user (top-level, cross-resource).
// GET /users/fetch-me → {sessionId, user:{id, email, firstName, ...}}.
func (s *Service) newMeCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:   "me",
		Short: "Get the current PhantomBuster user (GET /users/fetch-me)",
		Long: "Resolves the API key to its user and session, which makes it the cheapest\n" +
			"credential check — a revoked or mistyped key fails here before any Phantom\n" +
			"is launched. Workspace-level facts, the S3 folder and the quota, come from\n" +
			"`org get` and `org resources` instead.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			raw, err := s.call(cmd.Context(), key, http.MethodGet, "/users/fetch-me", nil, nil)
			if err != nil {
				return err
			}
			return s.emitObject(raw, nil)
		},
	}
}
