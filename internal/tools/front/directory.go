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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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

// newInboxListCmd is `inbox list` (GET /inboxes) — discover inboxes.
func (s *Service) newInboxListCmd(token string) *cobra.Command {
	return s.newSimpleListCmd(token, "List inboxes", "/inboxes")
}

// newTeammateListCmd is `teammate list` (GET /teammates) — resolve assignee
// targets.
func (s *Service) newTeammateListCmd(token string) *cobra.Command {
	return s.newSimpleListCmd(token, "List teammates", "/teammates")
}

// newTagListCmd is `tag list` (GET /tags) — resolve tag ids for tagging.
func (s *Service) newTagListCmd(token string) *cobra.Command {
	return s.newSimpleListCmd(token, "List tags", "/tags")
}

// newSimpleListCmd builds a paginated `list` command over a fixed path — the
// shared shape for inbox / teammate / tag, which take only limit / page-token.
func (s *Service) newSimpleListCmd(token, short, path string) *cobra.Command {
	var pageToken string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: short,
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
		Args:  cobra.NoArgs,
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
