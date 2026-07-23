package bluesky

import (
	"fmt"

	"github.com/spf13/cobra"
)

func shapeProfile(p rawProfile) profileView {
	return profileView{
		DID:            p.DID,
		Handle:         p.Handle,
		DisplayName:    p.DisplayName,
		Description:    p.Description,
		FollowersCount: p.FollowersCount,
		FollowsCount:   p.FollowsCount,
		PostsCount:     p.PostsCount,
	}
}

func (s *Service) newWhoamiCmd(sess *session) *cobra.Command {
	return &cobra.Command{
		Use:         "whoami",
		Short:       "Show the connected account (opens a session and reads the self profile)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if err := sess.ensure(ctx); err != nil {
				return err
			}
			prof, err := sess.getProfile(ctx, sess.did)
			if err != nil {
				return err
			}
			return s.emitValue(shapeProfile(prof))
		},
	}
}

func (s *Service) newProfileCmd(sess *session) *cobra.Command {
	cmd := &cobra.Command{Use: "profile", Short: "Profiles"}
	var actor string
	get := &cobra.Command{
		Use:         "get",
		Short:       "Get an actor's profile",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if actor == "" {
				return fmt.Errorf("--actor is required")
			}
			prof, err := sess.getProfile(cmd.Context(), actor)
			if err != nil {
				return err
			}
			return s.emitValue(shapeProfile(prof))
		},
	}
	get.Flags().StringVar(&actor, "actor", "", "handle or DID of the profile")
	cmd.AddCommand(get)
	return cmd
}
