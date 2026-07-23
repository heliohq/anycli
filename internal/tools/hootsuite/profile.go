package hootsuite

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newProfileListCmd lists the social profiles the token may post to.
func (s *Service) newProfileListCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "list",
		Short:       "List social profiles (GET /v1/socialProfiles)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/socialProfiles", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// newProfileGetCmd fetches one social profile by id.
func (s *Service) newProfileGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <id>",
		Short:       "Get one social profile (GET /v1/socialProfiles/{id})",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/socialProfiles/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// newProfileTeamsCmd lists the teams with access to a social profile.
func (s *Service) newProfileTeamsCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "teams <id>",
		Short:       "List teams with access to a social profile (GET /v1/socialProfiles/{id}/teams)",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/socialProfiles/"+url.PathEscape(args[0])+"/teams", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}
