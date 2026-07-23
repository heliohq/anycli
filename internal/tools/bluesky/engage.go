package bluesky

import (
	"github.com/spf13/cobra"
)

func (s *Service) newLikeCmd(sess *session) *cobra.Command {
	return s.newEngagementCmd(sess, "like", "Like a post", collectionLike)
}

func (s *Service) newRepostCmd(sess *session) *cobra.Command {
	return s.newEngagementCmd(sess, "repost", "Repost a post", collectionRepost)
}

// newEngagementCmd builds a like/repost command: both create a record whose
// subject is the target post's {uri, cid}.
func (s *Service) newEngagementCmd(sess *session, use, short, collection string) *cobra.Command {
	var uri, cid string
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := parseATURI(uri); err != nil {
				return err
			}
			record := map[string]any{
				"$type":     collection,
				"subject":   recordRef{URI: uri, CID: cid},
				"createdAt": nowRFC3339(),
			}
			resp, err := sess.createRecord(cmd.Context(), collection, record)
			if err != nil {
				return err
			}
			return s.emitValue(map[string]string{"uri": resp.URI, "cid": resp.CID, "subject_uri": uri})
		},
	}
	cmd.Flags().StringVar(&uri, "uri", "", "at:// URI of the target post")
	cmd.Flags().StringVar(&cid, "cid", "", "cid of the target post")
	_ = cmd.MarkFlagRequired("uri")
	_ = cmd.MarkFlagRequired("cid")
	return cmd
}
