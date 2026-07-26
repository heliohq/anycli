package mastodon

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// The four engagement Longs live next to the two shared builders because it is
// the builder that fixes the argument shape (a status id flag vs. an
// account handle) while the consequence of each action is entirely its own.
const (
	longFavourite = "A public like on a status: the author sees it and the count is visible to\n" +
		"everyone. `--id` is a status id from a timeline, `search` or\n" +
		"`notifications list`. There is no unfavourite verb here — undoing one means\n" +
		"`api POST /api/v1/statuses/<id>/unfavourite`."

	longBoost = "Republishes another account's status to this account's followers —\n" +
		"Mastodon's retweet, and read as an endorsement, since it carries the\n" +
		"original post to an audience that never chose to follow its author. `--id`\n" +
		"is the status id. There is no unboost verb here; undoing one means\n" +
		"`api POST /api/v1/statuses/<id>/unreblog`."

	longFollow = "Takes `@user@instance` or a numeric id and returns the Relationship object\n" +
		"verbatim — read it rather than assuming success. A LOCKED account turns\n" +
		"this into a request: `requested: true` with `following: false`, pending\n" +
		"until the person approves, which may be never. On most instances the\n" +
		"follower list is public, so following is itself a visible act."

	longUnfollow = "Takes `@user@instance` or a numeric id and returns the Relationship object,\n" +
		"where `following: false` is the confirmation. It also withdraws a follow\n" +
		"request that is still pending. It does NOT block or mute: the account can\n" +
		"still see, reply to and mention this one."
)

// newStatusActionCmd builds a --id-flagged status engagement command
// (favourite / boost) that POSTs /api/v1/statuses/:id/<action> and emits the
// returned status's compact shape.
func (rt *runContext) newStatusActionCmd(use, short, long, action string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Long:        long,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			id, _ := cmd.Flags().GetString("id")
			if id == "" {
				return &usageError{msg: use + " requires --id"}
			}
			path := "/api/v1/statuses/" + url.PathEscape(id) + "/" + action
			body, _, err := rt.call(cmd.Context(), http.MethodPost, path, nil, nil)
			if err != nil {
				return err
			}
			status, err := decodeStatus(body)
			if err != nil {
				return err
			}
			return rt.emitJSON(createdFromStatus(status))
		},
	}
	cmd.Flags().String("id", "", "status id (required)")
	return cmd
}

func (rt *runContext) newFavouriteCmd() *cobra.Command {
	return rt.newStatusActionCmd("favourite", "Favourite (like) a status", longFavourite, "favourite")
}

func (rt *runContext) newBoostCmd() *cobra.Command {
	return rt.newStatusActionCmd("boost", "Boost (reblog) a status", longBoost, "reblog")
}

// newAccountRelationCmd builds a follow/unfollow command that resolves the
// account handle-or-id and POSTs /api/v1/accounts/:id/<action>.
func (rt *runContext) newAccountRelationCmd(use, short, long, action string) *cobra.Command {
	return &cobra.Command{
		Use:         use + " <acct|id>",
		Short:       short,
		Long:        long,
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := rt.resolveAccountID(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			path := "/api/v1/accounts/" + url.PathEscape(id) + "/" + action
			body, _, err := rt.call(cmd.Context(), http.MethodPost, path, nil, nil)
			if err != nil {
				return err
			}
			// The response is a Relationship object; echo it verbatim so the AI
			// sees following/requested state.
			return rt.emitRaw(body)
		},
	}
}

func (rt *runContext) newFollowCmd() *cobra.Command {
	return rt.newAccountRelationCmd("follow", "Follow an account", longFollow, "follow")
}

func (rt *runContext) newUnfollowCmd() *cobra.Command {
	return rt.newAccountRelationCmd("unfollow", "Unfollow an account", longUnfollow, "unfollow")
}

// emitRaw writes a provider JSON body to stdout verbatim (plus a newline). Used
// where the provider shape is already agent-consumable (relationship objects).
func (rt *runContext) emitRaw(body []byte) error {
	_, err := rt.svc.stdout().Write(append(trimTrailingNewline(body), '\n'))
	return err
}

// trimTrailingNewline drops a single trailing newline so emitRaw's own newline
// is not doubled.
func trimTrailingNewline(b []byte) []byte {
	if n := len(b); n > 0 && b[n-1] == '\n' {
		return b[:n-1]
	}
	return b
}
