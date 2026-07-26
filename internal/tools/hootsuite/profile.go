package hootsuite

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newProfileListCmd lists the social profiles the token may post to.
func (s *Service) newProfileListCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List social profiles (GET /v1/socialProfiles)",
		Long: "The mandatory first step before posting. Each row is one connected account,\n" +
			"and its numeric `id` is what `message schedule --profile` requires — a network\n" +
			"name or an @handle is rejected locally. Each row's `type` (`TWITTER`,\n" +
			"`LINKEDIN`, `FACEBOOK`, `INSTAGRAM`, `PINTEREST`, …) also decides whether the\n" +
			"Pinterest-only flags on `message schedule` come into play.",
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
		Use:   "get <id>",
		Short: "Get one social profile (GET /v1/socialProfiles/{id})",
		Long: "Confirms that a profile id found elsewhere still points at the handle\n" +
			"expected — worth one call before scheduling to an id that did not come from a\n" +
			"fresh `profile list`, since posting to the wrong connected account is not\n" +
			"undoable once sent. Returns the profile's `type`, its social-network username\n" +
			"and its owning organization.",
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
		Use:   "teams <id>",
		Short: "List teams with access to a social profile (GET /v1/socialProfiles/{id}/teams)",
		Long: "Answers \"who else can post as this account\". Teams are an organization-level\n" +
			"construct, so a profile held personally rather than through an organization\n" +
			"generally has none. Read-only here — team membership and profile access are\n" +
			"managed in the Hootsuite web app, not through this API surface.",
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
