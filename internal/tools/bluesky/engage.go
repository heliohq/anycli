package bluesky

import (
	"github.com/spf13/cobra"
)

// longLike and longRepost are the two engagement Longs. They live next to the
// shared builder because it is the builder that fixes the {uri, cid} subject
// and the create-a-record mechanism both texts describe; only what the record
// means differs.
const (
	longLike = "Both --uri and --cid of the target post are required: the cid is a\n" +
		"content hash that every read returns beside the uri, and an invented or\n" +
		"stale one is rejected. There is no unlike command — the `uri` in the\n" +
		"response is the like record itself, and `post delete` on that URI\n" +
		"removes the like."

	longRepost = "Both --uri and --cid of the target post are required, exactly as for\n" +
		"`like`. This is a bare repost carrying no commentary; adding a remark\n" +
		"means `post create --quote <uri>`, which produces a separate post rather\n" +
		"than a repost. There is no unrepost command — pass the `uri` in the\n" +
		"response to `post delete` to undo it."
)

func (s *Service) newLikeCmd(sess *session) *cobra.Command {
	return s.newEngagementCmd(sess, "like", "Like a post", longLike, collectionLike)
}

func (s *Service) newRepostCmd(sess *session) *cobra.Command {
	return s.newEngagementCmd(sess, "repost", "Repost a post", longRepost, collectionRepost)
}

// newEngagementCmd builds a like/repost command: both create a record whose
// subject is the target post's {uri, cid}.
func (s *Service) newEngagementCmd(sess *session, use, short, long, collection string) *cobra.Command {
	var uri, cid string
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Long:        long,
		Args:        cobra.NoArgs,
		Annotations: writeAction,
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
