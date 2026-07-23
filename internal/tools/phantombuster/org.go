package phantombuster

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newOrgGetCmd fetches the current workspace/org identity.
// GET /orgs/fetch → raw object (id is a string).
func (s *Service) newOrgGetCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:         "get",
		Short:       "Get the current workspace/org identity (GET /orgs/fetch)",
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
		Use:         "resources",
		Short:       "Get org resources, usage, and remaining quota (GET /orgs/fetch-resources)",
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
		Use:         "me",
		Short:       "Get the current PhantomBuster user (GET /users/fetch-me)",
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
