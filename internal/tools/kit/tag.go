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
		Long: "The tag ids that `tag add`, `tag remove` and `broadcast create --tag-id`\n" +
			"all take; none of them accepts a tag name. One page per call; continue\n" +
			"with --after.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "--name is required and is all a tag has. Creating one is cheap, but there\n" +
			"is no delete or rename in this tool, so a mistyped tag stays in the\n" +
			"account's vocabulary until it is cleaned up in Kit's UI — check `tag list`\n" +
			"for an existing tag before adding a near-duplicate.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
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

// longTagAdd and longTagRemove are the two membership Longs. They live next to
// the shared builder because it is the builder that fixes the --tag-id plus
// id-XOR-email argument shape both describe.
const (
	longTagAdd = "Applying a tag is Kit's automation TRIGGER: any sequence or automation\n" +
		"watching this tag fires as a result, so this is frequently the real action\n" +
		"rather than `sequence add`, which enrols someone while skipping the tag.\n" +
		"--tag-id is required, plus exactly one of --subscriber-id or --email;\n" +
		"passing both or neither is a usage error. Tag ids come from `tag list` —\n" +
		"a tag name is not accepted."

	longTagRemove = "Removes the tag only. Whatever automation the tag already triggered is NOT\n" +
		"undone: a subscriber part-way through a sequence stays there, so this is\n" +
		"not a way to pull someone out of a funnel. --tag-id is required, plus\n" +
		"exactly one of --subscriber-id or --email. Removing by email sends the\n" +
		"address as a query parameter, since a DELETE carries no body."
)

// tagMembershipCmd builds `tag add` (add=true) or `tag remove` (add=false).
// Both target a subscriber by --subscriber-id XOR --email under a --tag-id.
func (s *Service) tagMembershipCmd(token string, add bool) *cobra.Command {
	use, short, long := "remove", "Remove a tag from a subscriber", longTagRemove
	if add {
		use, short, long = "add", "Add a tag to a subscriber", longTagAdd
	}
	var tagID, subscriberID int
	var email string
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Long:        long,
		Args:        cobra.NoArgs,
		Annotations: writeAction,
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
