package kit

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// tagCmd groups the tag (audience segmentation) commands. Tagging is Kit's
// automation-trigger primitive.
func (s *Service) tagCmd(token string) *cobra.Command {
	group := newGroupCmd("tag", "Manage tags and tag membership")
	group.AddCommand(
		s.tagListCmd(token),
		s.tagCreateCmd(token),
		s.tagMembershipCmd(token, true),
		s.tagMembershipCmd(token, false),
	)
	return group
}

func (s *Service) tagListCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tags (one page; use --after to continue)",
		Args:  cobra.NoArgs,
	}
	lf := registerListFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		lf.apply(q)
		body, err := s.call(cmd.Context(), token, http.MethodGet, "/tags", q, nil)
		if err != nil {
			return err
		}
		return s.emitData(body, "tags")
	}
	return cmd
}

func (s *Service) tagCreateCmd(token string) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a tag",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if name == "" {
				return &usageError{msg: "--name is required"}
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/tags", nil, map[string]any{"name": name})
			if err != nil {
				return err
			}
			return s.emitData(body, "tag")
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "tag name (required)")
	return cmd
}

// tagMembershipCmd builds `tag add` (add=true) or `tag remove` (add=false).
// Both target a subscriber by --subscriber-id XOR --email under a --tag-id.
func (s *Service) tagMembershipCmd(token string, add bool) *cobra.Command {
	use, short := "remove", "Remove a tag from a subscriber"
	if add {
		use, short = "add", "Add a tag to a subscriber"
	}
	var tagID, subscriberID int
	var email string
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requirePositive("tag-id", tagID); err != nil {
				return err
			}
			method := http.MethodPost
			if !add {
				method = http.MethodDelete
			}
			suffix, q, reqBody, err := membershipRequest(method, subscriberID, email)
			if err != nil {
				return err
			}
			path := "/tags/" + strconv.Itoa(tagID) + "/subscribers" + suffix
			var payload any
			if reqBody != nil {
				payload = reqBody
			}
			body, callErr := s.call(cmd.Context(), token, method, path, q, payload)
			if callErr != nil {
				return callErr
			}
			return s.emitData(body, "subscriber")
		},
	}
	cmd.Flags().IntVar(&tagID, "tag-id", 0, "tag id (required)")
	cmd.Flags().IntVar(&subscriberID, "subscriber-id", 0, "subscriber id (XOR --email)")
	cmd.Flags().StringVar(&email, "email", "", "subscriber email (XOR --subscriber-id)")
	return cmd
}
