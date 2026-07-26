package bluesky

import (
	"fmt"

	"github.com/spf13/cobra"
)

func (s *Service) newFollowCmd(sess *session) *cobra.Command {
	var actor string
	cmd := &cobra.Command{
		Use:   "follow",
		Short: "Follow an actor",
		Long: "--actor is a handle or DID and is resolved to a DID before the record is\n" +
			"written. The `uri` in the response is the follow RECORD's at:// URI, and\n" +
			"it is the only thing `unfollow` accepts — keep it, because nothing in\n" +
			"this tool finds an existing follow record from the actor it points at.",
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
		Use:   "unfollow",
		Short: "Unfollow by deleting the follow record (its at:// URI)",
		Long: "--uri is the at:// URI of the app.bsky.graph.follow RECORD, never the\n" +
			"followed actor's handle; a URI from any other collection is rejected\n" +
			"before the call. Only the `follow` that created the record produces that\n" +
			"URI, so an account followed elsewhere — in the Bluesky app, say — cannot\n" +
			"be unfollowed from here.",
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
