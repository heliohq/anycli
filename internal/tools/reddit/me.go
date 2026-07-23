package reddit

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

// newMeCmd is `reddit me`: the identity of the connected account.
func (s *Service) newMeCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "me",
		Short:       "Show the connected Reddit account",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.get(cmd.Context(), token, "/api/v1/me", url.Values{})
			if err != nil {
				return err
			}
			d, err := decodeThingData(body)
			if err != nil {
				return err
			}
			if jsonMode(cmd) {
				return s.emitValue(map[string]any{
					"id":            d.ID,
					"name":          d.Name,
					"link_karma":    d.LinkKarma,
					"comment_karma": d.CommentKarma,
					"created_utc":   d.CreatedUTC,
				})
			}
			return s.emitLine(fmt.Sprintf("u/%s (%s)\tlink_karma=%d comment_karma=%d",
				d.Name, d.ID, d.LinkKarma, d.CommentKarma))
		},
	}
}
