package front

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// newContactListCmd is `contact list` (GET /contacts). --q filters by name /
// handle server-side; limit / page-token paginate.
func (s *Service) newContactListCmd(token string) *cobra.Command {
	var q, pageToken string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List or search contacts",
		Long: "--q filters server-side by name or handle, so an email address can be\n" +
			"found without knowing the contact id. Looking one up by an exact address\n" +
			"is cheaper through `contact get --id alt:email:<address>`, which is a\n" +
			"direct read rather than a search. --limit is capped at 100; continue with\n" +
			"--page-token.",
		Args: cobra.NoArgs,
	}
	cmd.Annotations = readOnly
	cmd.Flags().StringVar(&q, "q", "", "filter by name or handle")
	cmd.Flags().IntVar(&limit, "limit", 0, "max results per page (Front caps at 100)")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "cursor from a prior response's next_page_token")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		query := pageQuery(limit, pageToken)
		if q != "" {
			query.Set("q", q)
		}
		body, err := s.call(cmd.Context(), token, http.MethodGet, "/contacts", query, nil)
		if err != nil {
			return err
		}
		return s.emitList(body)
	}
	return cmd
}

// newContactGetCmd is `contact get --id <cnt_id|alt:source:handle>`
// (GET /contacts/{id}). Front accepts a contact id or an alternate-handle alias
// like alt:email:jane@example.com.
func (s *Service) newContactGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a contact by id or handle alias",
		Long: "--id accepts either a Front contact id or a handle alias written\n" +
			"`alt:<source>:<value>` — `alt:email:jane@example.com`,\n" +
			"`alt:phone:+15551234567`. The alias form resolves an address straight to\n" +
			"the contact with no search step, which is the fastest way to answer \"do we\n" +
			"already know this person\".",
		Args: cobra.NoArgs,
	}
	cmd.Annotations = readOnly
	cmd.Flags().StringVar(&id, "id", "", "contact id or alt:source:handle (required)")
	_ = cmd.MarkFlagRequired("id")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		body, err := s.call(cmd.Context(), token, http.MethodGet, "/contacts/"+url.PathEscape(id), nil, nil)
		if err != nil {
			return err
		}
		return s.emitObject(body)
	}
	return cmd
}

// newContactCreateCmd is `contact create --handle <source:value>…`
// (POST /contacts). At least one --handle is required; each is a
// source:value pair (e.g. email:jane@example.com, phone:+15551234567). --name
// is optional.
func (s *Service) newContactCreateCmd(token string) *cobra.Command {
	var name string
	var handles []string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a contact",
		Long: "At least one --handle is required, written source:value —\n" +
			"`email:jane@example.com`, `phone:+15551234567` — and the flag is\n" +
			"repeatable so one contact can carry several. The split is on the first\n" +
			"colon, so the value keeps any further colons. A handle already attached to\n" +
			"another contact is rejected rather than merged, so check with `contact get\n" +
			"--id alt:email:<address>` first.",
		Args: cobra.NoArgs,
	}
	cmd.Annotations = writeAction
	cmd.Flags().StringVar(&name, "name", "", "contact display name")
	cmd.Flags().StringArrayVar(&handles, "handle", nil, "source:value handle, e.g. email:jane@example.com (repeatable, at least one required)")
	_ = cmd.MarkFlagRequired("handle")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		parsed, err := parseHandles(handles)
		if err != nil {
			return err
		}
		payload := map[string]any{"handles": parsed}
		if name != "" {
			payload["name"] = name
		}
		body, err := s.call(cmd.Context(), token, http.MethodPost, "/contacts", nil, payload)
		if err != nil {
			return err
		}
		return s.emitObject(body)
	}
	return cmd
}

// parseHandles turns source:value strings into Front handle objects
// ({"source":…,"handle":…}). The split is on the first colon so a value that
// itself contains a colon (an email is fine, but be safe) is preserved.
func parseHandles(raw []string) ([]map[string]string, error) {
	out := make([]map[string]string, 0, len(raw))
	for _, h := range raw {
		source, value, ok := strings.Cut(h, ":")
		if !ok || source == "" || value == "" {
			return nil, &usageError{msg: `--handle must be source:value, e.g. email:jane@example.com`}
		}
		out = append(out, map[string]string{"source": source, "handle": value})
	}
	return out, nil
}

// longInboxList, longTeammateList and longTagList are the three lookup Longs.
// They live next to the shared builder because it is the builder that fixes
// the limit / page-token surface all three describe.
const (
	longInboxList = "The inbox ids `conversation list --inbox` and `conversation update --inbox`\n" +
		"take, plus the channels attached to each inbox — a channel id from here is\n" +
		"what `draft create --channel` requires. An inbox is a queue, a channel is\n" +
		"the address messages leave from; they are different ids. --limit is capped\n" +
		"at 100; continue with --page-token."

	longTeammateList = "The teammate ids --assignee on `conversation update` and --author on\n" +
		"`message send`, `draft create` and `comment add` accept. Names are never\n" +
		"accepted by those flags, so this is the only way to turn a person into an\n" +
		"id. --limit is capped at 100; continue with --page-token."

	longTagList = "The tag ids `conversation update --tag-add` and --tag-remove take; both\n" +
		"reject tag names. Tags cannot be created, renamed or deleted through this\n" +
		"tool, so a tag that is not in this list has to be made in Front first.\n" +
		"--limit is capped at 100; continue with --page-token."
)

// newInboxListCmd is `inbox list` (GET /inboxes) — discover inboxes.
func (s *Service) newInboxListCmd(token string) *cobra.Command {
	return s.newSimpleListCmd(token, "List inboxes", longInboxList, "/inboxes")
}

// newTeammateListCmd is `teammate list` (GET /teammates) — resolve assignee
// targets.
func (s *Service) newTeammateListCmd(token string) *cobra.Command {
	return s.newSimpleListCmd(token, "List teammates", longTeammateList, "/teammates")
}

// newTagListCmd is `tag list` (GET /tags) — resolve tag ids for tagging.
func (s *Service) newTagListCmd(token string) *cobra.Command {
	return s.newSimpleListCmd(token, "List tags", longTagList, "/tags")
}

// newSimpleListCmd builds a paginated `list` command over a fixed path — the
// shared shape for inbox / teammate / tag, which take only limit / page-token.
func (s *Service) newSimpleListCmd(token, short, long, path string) *cobra.Command {
	var pageToken string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: short,
		Long:  long,
		Args:  cobra.NoArgs,
	}
	cmd.Annotations = readOnly
	cmd.Flags().IntVar(&limit, "limit", 0, "max results per page (Front caps at 100)")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "cursor from a prior response's next_page_token")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		body, err := s.call(cmd.Context(), token, http.MethodGet, path, pageQuery(limit, pageToken), nil)
		if err != nil {
			return err
		}
		return s.emitList(body)
	}
	return cmd
}

// newMeCmd is `me` (GET /me) — the Front company the token is scoped to; the
// connection-identity source and a handy debug read.
func (s *Service) newMeCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "me",
		Short: "Show the Front company this token is scoped to",
		Long: "Names the single Front company this connection reaches. Worth running\n" +
			"first when a conversation, inbox or teammate cannot be found: a connection\n" +
			"covers one company only, so the usual cause is the wrong company rather\n" +
			"than a missing permission.",
		Args: cobra.NoArgs,
	}
	cmd.Annotations = readOnly
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		body, err := s.call(cmd.Context(), token, http.MethodGet, "/me", nil, nil)
		if err != nil {
			return err
		}
		return s.emitObject(body)
	}
	return cmd
}
