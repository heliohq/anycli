package bluesky

import (
	"fmt"

	"github.com/spf13/cobra"
)

func (s *Service) newFollowCmd(sess *session) *cobra.Command {
	var actor string
	cmd := &cobra.Command{
		Use:         "follow",
		Short:       "Follow an actor",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if actor == "" {
				return fmt.Errorf("--actor is required")
			}
			ctx := cmd.Context()
			did, err := sess.resolveActorDID(ctx, actor)
			if err != nil {
				return err
			}
			record := map[string]any{
				"$type":     collectionFollow,
				"subject":   did,
				"createdAt": nowRFC3339(),
			}
			resp, err := sess.createRecord(ctx, collectionFollow, record)
			if err != nil {
				return err
			}
			return s.emitValue(map[string]string{"uri": resp.URI, "cid": resp.CID, "subject": did})
		},
	}
	cmd.Flags().StringVar(&actor, "actor", "", "handle or DID of the actor to follow")
	return cmd
}

func (s *Service) newUnfollowCmd(sess *session) *cobra.Command {
	var uri string
	cmd := &cobra.Command{
		Use:         "unfollow",
		Short:       "Unfollow by deleting the follow record (its at:// URI)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			parsed, err := parseATURI(uri)
			if err != nil {
				return err
			}
			if parsed.Collection != collectionFollow {
				return fmt.Errorf("--uri must be an %s record", collectionFollow)
			}
			if err := sess.deleteRecord(cmd.Context(), parsed); err != nil {
				return err
			}
			return s.emitValue(map[string]string{"uri": uri, "deleted": "true"})
		},
	}
	cmd.Flags().StringVar(&uri, "uri", "", "at:// URI of the follow record to delete")
	_ = cmd.MarkFlagRequired("uri")
	return cmd
}
