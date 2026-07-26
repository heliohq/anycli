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
		Long: "The connected token's workspace id, name and slug, plus\n" +
			"`authorized_by_workspace_member_id` — the member `comment create` uses as\n" +
			"its default author. Worth running when a comment fails for want of an\n" +
			"author, or to confirm which workspace's schema `object list` is about to\n" +
			"describe.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "The first call of any session that writes, because object slugs are\n" +
			"per-workspace and include custom objects. Only `people` and `companies`\n" +
			"exist in every workspace; `deals`, `users` and `workspaces` are optional\n" +
			"standard objects an admin has to enable, and a Free-plan workspace caps at\n" +
			"three objects in total. The slugs here are what the `record` commands,\n" +
			"`attribute list --object` and `record search --objects` take.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "Takes a slug or an id and returns the object's own definition — its\n" +
			"singular and plural names and its id. It does NOT return the object's\n" +
			"attributes, which is what a --values payload actually needs; those come\n" +
			"from `attribute list --object <slug>`.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
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
		Long: "Exactly one of --object or --list — neither or both is a usage error. This\n" +
			"is the lookup that makes a --values payload correct: it names the\n" +
			"attribute slugs, their types, and which are unique, and only a unique\n" +
			"attribute can be used as `record upsert --match`. A list has its OWN\n" +
			"attributes, separate from its parent object's, and those are the ones\n" +
			"`entry add --values` and `entry update` write. Paged by --limit/--offset\n" +
			"as query params.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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

// longAttributeOptions and longAttributeStatuses are the two attribute-child
// Longs. They live next to the shared builder because it is the builder that
// fixes the --object/--list pair and the required --attribute both describe,
// while the attribute TYPE each one applies to is what tells them apart.
const (
	longAttributeOptions = "For a SELECT attribute: the option titles and ids a write is allowed to\n" +
		"use. Requires --attribute plus exactly one of --object or --list. A select\n" +
		"value outside this set is rejected, and the options are workspace-specific,\n" +
		"so read them rather than copying values seen on another workspace's\n" +
		"records."

	longAttributeStatuses = "For a STATUS attribute: the stages it can hold, with their ids and order —\n" +
		"deal stages being the usual case. Requires --attribute plus exactly one of\n" +
		"--object or --list. Status and select are different attribute types, so an\n" +
		"attribute that returns nothing here is probably a select and belongs to\n" +
		"`attribute options`."
)

// newAttributeOptionsCmd is `attribute options --object <o> | --list <l>
// --attribute <a>` (GET /v2/{target}/{identifier}/attributes/{attribute}/options):
// list a select attribute's options so a write can use a valid option.
func (s *Service) newAttributeOptionsCmd(token string) *cobra.Command {
	return s.newAttributeChildCmd(token, "options", "List a select attribute's options", longAttributeOptions)
}

// newAttributeStatusesCmd is `attribute statuses …/statuses`: list a status
// attribute's stages (e.g. deal stages).
func (s *Service) newAttributeStatusesCmd(token string) *cobra.Command {
	return s.newAttributeChildCmd(token, "statuses", "List a status attribute's stages", longAttributeStatuses)
}

// newAttributeChildCmd builds the shared options/statuses subcommand: both hang
// off /v2/{target}/{identifier}/attributes/{attribute}/{child} and differ only
// in the trailing path segment.
func (s *Service) newAttributeChildCmd(token, child, short, long string) *cobra.Command {
	var object, list, attribute string
	cmd := &cobra.Command{
		Use:         child,
		Short:       short,
		Long:        long,
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "Workspace members are the actor ids the tool uses: --assignee on `task\n" +
			"create` and `task update`, --author on `comment create`, and\n" +
			"--request-as-member on `record search`. They are NOT records in the\n" +
			"`people` object — a colleague can exist as both, under different ids, and\n" +
			"passing a people-record id where a member id is expected fails.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "Takes the workspace-member UUID from `member list`, or one read off a\n" +
			"record's created_by or owner value, and resolves it to a name and email.\n" +
			"This is the resolution step for any actor id another response returns.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
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
