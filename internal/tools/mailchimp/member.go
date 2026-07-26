package mailchimp

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// newMemberCmd builds the member group: list, get, upsert, archive, tag.
func (s *Service) newMemberCmd(r *requester) *cobra.Command {
	group := newGroupCmd("member", "Manage audience members (subscribers)")
	group.AddCommand(
		s.newMemberListCmd(r),
		s.newMemberGetCmd(r),
		s.newMemberUpsertCmd(r),
		s.newMemberArchiveCmd(r),
		s.newMemberTagCmd(r),
	)
	return group
}

func (s *Service) newMemberListCmd(r *requester) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <list_id>",
		Short: "List members (GET /lists/{list_id}/members)",
		Long: "`--status` filters server-side and is usually worth setting: the rows include\n" +
			"`unsubscribed`, `cleaned` and `archived` members, so an unfiltered count is\n" +
			"not the mailable count. Accepted values are `subscribed`, `unsubscribed`,\n" +
			"`cleaned`, `pending`, `transactional` and `archived`. For a single known\n" +
			"address use `member get --email`, which is one call instead of paging.",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := listQuery(cmd)
			if status, _ := cmd.Flags().GetString("status"); status != "" {
				q.Set("status", status)
			}
			body, err := r.do(cmd.Context(), http.MethodGet, "/lists/"+url.PathEscape(args[0])+"/members", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	registerListFlags(cmd)
	cmd.Flags().String("status", "", "filter by status: subscribed|unsubscribed|cleaned|pending|transactional|archived")
	return cmd
}

func (s *Service) newMemberGetCmd(r *requester) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <list_id>",
		Short: "Get one member (GET /lists/{list_id}/members/{subscriber_hash})",
		Long: "Requires exactly one of `--email` (hashed to the subscriber hash locally) or\n" +
			"`--hash`; neither or both is a usage error raised before any request. The\n" +
			"list id is the positional argument and the member selector is a flag, not the\n" +
			"other way round. An address that was never in this audience answers 404,\n" +
			"which is the intended membership test — there is no separate \"is this address\n" +
			"subscribed\" command.",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			hash, err := memberSelector(cmd, "member get")
			if err != nil {
				return err
			}
			body, err := r.do(cmd.Context(), http.MethodGet, memberPath(args[0], hash), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	registerMemberSelectorFlags(cmd)
	return cmd
}

func (s *Service) newMemberUpsertCmd(r *requester) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upsert <list_id>",
		Short: "Add or update a member (PUT /lists/{list_id}/members/{subscriber_hash})",
		Long: "A PUT, so it creates or updates in one idempotent call. `--status-if-new`\n" +
			"(default `subscribed`) applies ONLY at creation while `--status` changes an\n" +
			"existing member, so passing just one of them silently does nothing in the\n" +
			"other case. Mailchimp refuses to re-subscribe an address that previously\n" +
			"unsubscribed or was cleaned — that is an API error, and the person has to opt\n" +
			"in themselves. `--merge` is a JSON object keyed by merge tag\n" +
			"(`{\"FNAME\":\"Ada\"}`); `--tags` is honored on creation, and `member tag` is the\n" +
			"way to change tags afterwards.",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			email, _ := cmd.Flags().GetString("email")
			if email == "" {
				return &usageError{msg: "member upsert requires --email"}
			}
			payload := map[string]any{"email_address": email}
			statusIfNew, _ := cmd.Flags().GetString("status-if-new")
			payload["status_if_new"] = statusIfNew
			if status, _ := cmd.Flags().GetString("status"); status != "" {
				payload["status"] = status
			}
			if merge, _ := cmd.Flags().GetString("merge"); merge != "" {
				var mf map[string]any
				if err := json.Unmarshal([]byte(merge), &mf); err != nil {
					return &usageError{msg: "--merge is not valid JSON: " + err.Error()}
				}
				payload["merge_fields"] = mf
			}
			if tags, _ := cmd.Flags().GetString("tags"); tags != "" {
				payload["tags"] = splitCSV(tags)
			}
			body, err := r.do(cmd.Context(), http.MethodPut, memberPath(args[0], subscriberHash(email)), nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().String("email", "", "member email address (required)")
	cmd.Flags().String("status", "", "set status for an existing member: subscribed|unsubscribed|cleaned|pending")
	cmd.Flags().String("status-if-new", "subscribed", "status applied when the member is newly created")
	cmd.Flags().String("merge", "", "merge fields as a JSON object, e.g. {\"FNAME\":\"Ada\"}")
	cmd.Flags().String("tags", "", "comma-separated tags to set on a new member")
	return cmd
}

func (s *Service) newMemberArchiveCmd(r *requester) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "archive <list_id>",
		Short: "Archive a member (DELETE /lists/{list_id}/members/{subscriber_hash})",
		Long: "Archiving is Mailchimp's soft delete: the member stops being mailable and\n" +
			"drops out of the default member view, but their history survives and they can\n" +
			"be restored from the web app. It is NOT an unsubscribe — no opt-out is\n" +
			"recorded — and it is not the permanent delete, which this tool deliberately\n" +
			"does not expose. `member list --status archived` finds them again.",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			hash, err := memberSelector(cmd, "member archive")
			if err != nil {
				return err
			}
			if _, err := r.do(cmd.Context(), http.MethodDelete, memberPath(args[0], hash), nil, nil); err != nil {
				return err
			}
			return s.emitValue(actionReceipt("archive", hash))
		},
	}
	registerMemberSelectorFlags(cmd)
	return cmd
}

func (s *Service) newMemberTagCmd(r *requester) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tag <list_id>",
		Short: "Add/remove member tags (POST /lists/{list_id}/members/{subscriber_hash}/tags)",
		Long: "At least one of `--add` or `--remove` is required; both take comma-separated\n" +
			"names. Adding a name that does not exist yet creates the tag. Only the names\n" +
			"passed are touched — tags left unmentioned are preserved, so this is a delta,\n" +
			"not a replacement. Tags are scoped to one audience, so the same name in two\n" +
			"audiences is two independent tags.",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			hash, err := memberSelector(cmd, "member tag")
			if err != nil {
				return err
			}
			add, _ := cmd.Flags().GetString("add")
			remove, _ := cmd.Flags().GetString("remove")
			if add == "" && remove == "" {
				return &usageError{msg: "member tag requires at least one of --add or --remove"}
			}
			tags := make([]map[string]string, 0)
			for _, name := range splitCSV(add) {
				tags = append(tags, map[string]string{"name": name, "status": "active"})
			}
			for _, name := range splitCSV(remove) {
				tags = append(tags, map[string]string{"name": name, "status": "inactive"})
			}
			payload := map[string]any{"tags": tags}
			if _, err := r.do(cmd.Context(), http.MethodPost, memberPath(args[0], hash)+"/tags", nil, payload); err != nil {
				return err
			}
			return s.emitValue(actionReceipt("tag", hash))
		},
	}
	registerMemberSelectorFlags(cmd)
	cmd.Flags().String("add", "", "comma-separated tags to add")
	cmd.Flags().String("remove", "", "comma-separated tags to remove")
	return cmd
}

// registerMemberSelectorFlags wires the shared --email / --hash member
// selectors onto a command.
func registerMemberSelectorFlags(cmd *cobra.Command) {
	cmd.Flags().String("email", "", "member email address (hashed client-side)")
	cmd.Flags().String("hash", "", "subscriber hash (MD5 of the lowercase email)")
}

// memberPath builds the members subpath for a list id + subscriber hash.
func memberPath(listID, hash string) string {
	return "/lists/" + url.PathEscape(listID) + "/members/" + url.PathEscape(hash)
}

// splitCSV splits a comma-separated flag value, trimming whitespace and
// dropping empty entries.
func splitCSV(v string) []string {
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
