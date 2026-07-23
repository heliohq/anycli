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
		s.newTagApplyCmd(token, "apply", "applyTags", "Apply a tag to contacts (POST /v2/tags/{id}/contacts:applyTags)"),
		s.newTagApplyCmd(token, "remove", "removeTags", "Remove a tag from contacts (POST /v2/tags/{id}/contacts:removeTags)"),
	)
	return cmd
}

func (s *Service) newTagListCmd(token string) *cobra.Command {
	var lf *listFlags
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List tags (GET /v2/tags)",
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
		Use:         "get <tag-id>",
		Short:       "Get a tag (GET /v2/tags/{id})",
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
		Use:         "create",
		Short:       "Create a tag (POST /v2/tags)",
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
		Use:         "contacts <tag-id>",
		Short:       "List contacts with a tag (GET /v2/tags/{id}/contacts)",
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

// newTagApplyCmd builds the apply/remove verbs, which share the {contact_ids}
// body shape and differ only in the custom-verb path suffix.
func (s *Service) newTagApplyCmd(token, use, verb, short string) *cobra.Command {
	var contactIDs []string
	cmd := &cobra.Command{
		Use:         use + " <tag-id>",
		Short:       short,
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
