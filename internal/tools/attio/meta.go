package attio

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newWhoamiCmd is `whoami` (GET /v2/self): the token/workspace identity — also
// the bundle's identity endpoint. Output is the raw self payload under --json,
// a one-line workspace summary otherwise.
func (s *Service) newWhoamiCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Show the token's workspace identity",
		Args:  cobra.NoArgs,
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		id, body, err := s.self(cmd.Context(), token)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		if jsonMode {
			return s.emitJSON(body)
		}
		fmt.Fprintf(s.stdout(), "%s  %s (%s)\n", id.WorkspaceID, id.WorkspaceName, id.WorkspaceSlug)
		return nil
	}
	return cmd
}

// newObjectListCmd is `object list` (GET /v2/objects): discover object slugs,
// including custom objects, before any record op.
func (s *Service) newObjectListCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all objects (standard and custom)",
		Args:  cobra.NoArgs,
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		body, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/objects", nil)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// newObjectGetCmd is `object get <object>` (GET /v2/objects/{object}).
func (s *Service) newObjectGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <object>",
		Short: "Get one object by slug or id",
		Args:  cobra.ExactArgs(1),
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		body, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/objects/"+url.PathEscape(args[0]), nil)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// attributeTarget resolves the --object / --list flag pair into the
// /v2/{target}/{identifier} path prefix. Exactly one must be set.
func attributeTarget(object, list string) (string, error) {
	switch {
	case object != "" && list != "":
		return "", &usageError{msg: "pass exactly one of --object or --list, not both"}
	case object != "":
		return "/v2/objects/" + url.PathEscape(object), nil
	case list != "":
		return "/v2/lists/" + url.PathEscape(list), nil
	default:
		return "", &usageError{msg: "one of --object or --list is required"}
	}
}

// newAttributeListCmd is `attribute list --object <o> | --list <l>`
// (GET /v2/{target}/{identifier}/attributes): discover attribute slugs — the
// prerequisite for constructing correct write payloads.
func (s *Service) newAttributeListCmd(token string) *cobra.Command {
	var object, list string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List attributes on an object or a list",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().StringVar(&object, "object", "", "object slug or id (mutually exclusive with --list)")
	cmd.Flags().StringVar(&list, "list", "", "list slug or id (mutually exclusive with --object)")
	lo := registerLimitOffset(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		prefix, err := attributeTarget(object, list)
		if err != nil {
			return err
		}
		q := url.Values{}
		lo.applyToQuery(q)
		path := prefix + "/attributes"
		if enc := q.Encode(); enc != "" {
			path += "?" + enc
		}
		body, err := s.call(cmd.Context(), token, http.MethodGet, path, nil)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// newAttributeOptionsCmd is `attribute options --object <o> | --list <l>
// --attribute <a>` (GET /v2/{target}/{identifier}/attributes/{attribute}/options):
// list a select attribute's options so a write can use a valid option.
func (s *Service) newAttributeOptionsCmd(token string) *cobra.Command {
	return s.newAttributeChildCmd(token, "options", "List a select attribute's options")
}

// newAttributeStatusesCmd is `attribute statuses …/statuses`: list a status
// attribute's stages (e.g. deal stages).
func (s *Service) newAttributeStatusesCmd(token string) *cobra.Command {
	return s.newAttributeChildCmd(token, "statuses", "List a status attribute's stages")
}

// newAttributeChildCmd builds the shared options/statuses subcommand: both hang
// off /v2/{target}/{identifier}/attributes/{attribute}/{child} and differ only
// in the trailing path segment.
func (s *Service) newAttributeChildCmd(token, child, short string) *cobra.Command {
	var object, list, attribute string
	cmd := &cobra.Command{
		Use:   child,
		Short: short,
		Args:  cobra.NoArgs,
	}
	cmd.Flags().StringVar(&object, "object", "", "object slug or id (mutually exclusive with --list)")
	cmd.Flags().StringVar(&list, "list", "", "list slug or id (mutually exclusive with --object)")
	cmd.Flags().StringVar(&attribute, "attribute", "", "attribute slug or id (required)")
	_ = cmd.MarkFlagRequired("attribute")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		prefix, err := attributeTarget(object, list)
		if err != nil {
			return err
		}
		path := prefix + "/attributes/" + url.PathEscape(attribute) + "/" + child
		body, err := s.call(cmd.Context(), token, http.MethodGet, path, nil)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// newMemberListCmd is `member list` (GET /v2/workspace_members): resolve
// assignee/actor ids for tasks, notes and comment authors.
func (s *Service) newMemberListCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workspace members",
		Args:  cobra.NoArgs,
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		body, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/workspace_members", nil)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}

// newMemberGetCmd is `member get <member_id>` (GET /v2/workspace_members/{id}).
func (s *Service) newMemberGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <member_id>",
		Short: "Get one workspace member by id",
		Args:  cobra.ExactArgs(1),
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		body, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/workspace_members/"+url.PathEscape(args[0]), nil)
		if err != nil {
			return err
		}
		jsonMode, _ := cmd.Flags().GetBool("json")
		return s.emit(jsonMode, body)
	}
	return cmd
}
