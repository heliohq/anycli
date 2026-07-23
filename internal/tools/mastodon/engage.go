package mastodon

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newStatusActionCmd builds a --id-flagged status engagement command
// (favourite / boost) that POSTs /api/v1/statuses/:id/<action> and emits the
// returned status's compact shape.
func (rt *runContext) newStatusActionCmd(use, short, action string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
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
	return rt.newStatusActionCmd("favourite", "Favourite (like) a status", "favourite")
}

func (rt *runContext) newBoostCmd() *cobra.Command {
	return rt.newStatusActionCmd("boost", "Boost (reblog) a status", "reblog")
}

// newAccountRelationCmd builds a follow/unfollow command that resolves the
// account handle-or-id and POSTs /api/v1/accounts/:id/<action>.
func (rt *runContext) newAccountRelationCmd(use, short, action string) *cobra.Command {
	return &cobra.Command{
		Use:         use + " <acct|id>",
		Short:       short,
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
	return rt.newAccountRelationCmd("follow", "Follow an account", "follow")
}

func (rt *runContext) newUnfollowCmd() *cobra.Command {
	return rt.newAccountRelationCmd("unfollow", "Unfollow an account", "unfollow")
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
