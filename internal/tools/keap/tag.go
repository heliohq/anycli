package keap

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newTagCmd(token string) *cobra.Command {
	cmd := newGroupCmd("tag", "Tags (list, get, create, list contacts, apply, remove)")
	cmd.AddCommand(
		s.newTagListCmd(token),
		s.newTagGetCmd(token),
		s.newTagCreateCmd(token),
		s.newTagContactsCmd(token),
		s.newTagApplyCmd(token, "apply", "applyTags", "Apply a tag to contacts (POST /v2/tags/{id}/contacts:applyTags)", longTagApply),
		s.newTagApplyCmd(token, "remove", "removeTags", "Remove a tag from contacts (POST /v2/tags/{id}/contacts:removeTags)", longTagRemove),
	)
	return cmd
}

func (s *Service) newTagListCmd(token string) *cobra.Command {
	var lf *listFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tags (GET /v2/tags)",
		Long: "Tags are the account's segmentation primitive; the numeric id returned here\n" +
			"is what `tag get`, `tag contacts`, `tag apply` and `tag remove` all take.\n" +
			"`--filter name==VIP` finds one without paging the whole set.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/tags", lf.values(), nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	lf = registerListFlags(cmd)
	return cmd
}

func (s *Service) newTagGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <tag-id>",
		Short: "Get a tag (GET /v2/tags/{id})",
		Long: "Returns the tag's own record — name, description, category — and NOT its\n" +
			"members. For the contacts carrying it use `tag contacts <tag-id>`. This is\n" +
			"the one read verb in the group with no `--fields` or pagination flags.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/tags/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}

func (s *Service) newTagCreateCmd(token string) *cobra.Command {
	var name, description, jsonBody string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a tag (POST /v2/tags)",
		Long: "`--name` is required and enforced locally; `name` inside `--json-body` also\n" +
			"satisfies it. `--description` is the only other flag — a tag category is set\n" +
			"by putting `category` in `--json-body`, which is merged over the flags.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{}
			if name != "" {
				body["name"] = name
			}
			if description != "" {
				body["description"] = description
			}
			if err := applyJSONBody(body, jsonBody); err != nil {
				return err
			}
			if _, ok := body["name"]; !ok {
				return &usageError{msg: "tag create requires --name (or name in --json-body)"}
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/v2/tags", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "tag name (required)")
	cmd.Flags().StringVar(&description, "description", "", "tag description")
	cmd.Flags().StringVar(&jsonBody, "json-body", "", "raw JSON body merged over the flag-built payload (e.g. category)")
	return cmd
}

func (s *Service) newTagContactsCmd(token string) *cobra.Command {
	var lf *listFlags
	cmd := &cobra.Command{
		Use:   "contacts <tag-id>",
		Short: "List contacts with a tag (GET /v2/tags/{id}/contacts)",
		Long: "Lists the contacts carrying the tag, one page at a time, with the shared\n" +
			"`--page-size` / `--page-token` / `--fields` params. There is no inverse\n" +
			"verb: to see the tags on a single contact, read the contact record itself.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/tags/"+url.PathEscape(args[0])+"/contacts", lf.values(), nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	lf = registerListFlags(cmd)
	return cmd
}

// longTagApply and longTagRemove are the membership Longs. They live next to
// the shared builder because it is the builder that fixes the argument (a tag
// id) and the repeatable --contact-id body both describe.
const (
	longTagApply = "The tag id is the positional argument and the members are repeatable\n" +
		"`--contact-id` flags — at least one is required — so a batch is one call.\n" +
		"Tagging is Keap's usual automation trigger: applying a tag can start a\n" +
		"sequence that emails the contact or creates tasks, so check what is keyed on\n" +
		"the tag before bulk-applying it."
	longTagRemove = "The tag id is the positional argument and repeatable `--contact-id` flags\n" +
		"name the members to drop; at least one is required. Removing a tag is not an\n" +
		"undo — anything its application already triggered has already run, and\n" +
		"contacts already inside a sequence are not withdrawn from it."
)

// newTagApplyCmd builds the apply/remove verbs, which share the {contact_ids}
// body shape and differ only in the custom-verb path suffix.
func (s *Service) newTagApplyCmd(token, use, verb, short, long string) *cobra.Command {
	var contactIDs []string
	cmd := &cobra.Command{
		Use:         use + " <tag-id>",
		Short:       short,
		Long:        long,
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(contactIDs) == 0 {
				return &usageError{msg: "at least one --contact-id is required"}
			}
			body := map[string]any{"contact_ids": contactIDs}
			path := "/v2/tags/" + url.PathEscape(args[0]) + "/contacts:" + verb
			resp, err := s.call(cmd.Context(), token, http.MethodPost, path, nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringArrayVar(&contactIDs, "contact-id", nil, "contact id (repeatable)")
	return cmd
}
