package gorgias

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// This file holds the read-only directory/reporting resources: agents (users),
// tags, views, satisfaction surveys, and the account identity anchor.

// longUserList, longUserGet, longTagList and longSatisfactionList are the
// directory Longs. They live next to the two shared builders because it is the
// builder that fixes the fixed collection path and the cursor flags they
// describe.
const (
	longUserList = "Agents, not customers — the two are separate resources and separate id\n" +
		"spaces. The numeric ids here are what `ticket update --assignee` takes,\n" +
		"which accepts neither an email nor a name, so this is the only way to turn\n" +
		"a person into an assignee. Cursor-paged: continue with --cursor."

	longUserGet = "Takes the numeric agent id from `user list`; there is no lookup by email\n" +
		"or name. Useful for turning the assignee id on a ticket back into a human\n" +
		"the reply can be addressed to."

	longTagList = "The tags this helpdesk defines. `ticket update --tag` is keyed by tag NAME\n" +
		"rather than id — unlike --assignee — so what matters here is the exact\n" +
		"spelling. Tags cannot be created, renamed or deleted through this tool,\n" +
		"and a name it does not recognise is not silently created."

	longSatisfactionList = "CSAT survey results, read-only: nothing here sends a survey or changes a\n" +
		"score. Each row links back to the ticket that was rated, which is how a\n" +
		"low score is traced to the conversation that produced it. Cursor-paged:\n" +
		"continue with --cursor."
)

func (s *Service) newUserCmd(token, base string) *cobra.Command {
	cmd := newGroupCmd("user", "Resolve agents (list, get)")
	cmd.AddCommand(
		s.newSimpleListCmd(token, base, "list", "List agents (GET /users)", longUserList, "/users"),
		s.newSimpleGetCmd(token, base, "get <user-id>", "Retrieve an agent (GET /users/{id})", longUserGet, "/users/"),
	)
	return cmd
}

func (s *Service) newTagCmd(token, base string) *cobra.Command {
	cmd := newGroupCmd("tag", "Tag lookup for triage")
	cmd.AddCommand(
		s.newSimpleListCmd(token, base, "list", "List tags (GET /tags)", longTagList, "/tags"),
	)
	return cmd
}

func (s *Service) newSatisfactionCmd(token, base string) *cobra.Command {
	cmd := newGroupCmd("satisfaction", "CSAT survey reporting")
	cmd.AddCommand(
		s.newSimpleListCmd(token, base, "list", "List satisfaction surveys (GET /satisfaction-surveys)", longSatisfactionList, "/satisfaction-surveys"),
	)
	return cmd
}

func (s *Service) newViewCmd(token, base string) *cobra.Command {
	cmd := newGroupCmd("view", "The saved queues agents work from")
	cmd.AddCommand(
		s.newViewListCmd(token, base),
		s.newViewItemsCmd(token, base),
	)
	return cmd
}

func (s *Service) newAccountCmd(token, base string) *cobra.Command {
	cmd := newGroupCmd("account", "Account identity / health-check")
	cmd.AddCommand(&cobra.Command{
		Use:   "get",
		Short: "Retrieve the account (GET /account)",
		Long: "Takes no arguments and confirms two things at once: that the token still\n" +
			"works, and which Gorgias helpdesk this connection is bound to. A\n" +
			"connection reaches one subdomain only, so a ticket or customer that cannot\n" +
			"be found is often the wrong account rather than a deleted record.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, base, http.MethodGet, "/account", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	})
	return cmd
}

// newSimpleListCmd builds a paginated GET-list command on a fixed collection
// path (users, tags, satisfaction surveys) with the shared cursor flags.
func (s *Service) newSimpleListCmd(token, base, use, short, long, path string) *cobra.Command {
	var page pageFlags
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Long:        long,
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			page.apply(q)
			resp, err := s.call(cmd.Context(), token, base, http.MethodGet, path, q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	page.register(cmd)
	return cmd
}

// newSimpleGetCmd builds a single-resource GET command; pathPrefix ends with a
// trailing slash and the id is appended (path-escaped).
func (s *Service) newSimpleGetCmd(token, base, use, short, long, pathPrefix string) *cobra.Command {
	return &cobra.Command{
		Use:         use,
		Short:       short,
		Long:        long,
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, base, http.MethodGet, pathPrefix+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newViewListCmd(token, base string) *cobra.Command {
	var page pageFlags
	var category string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List views (GET /views)",
		Long: "Views are the saved queues agents actually work, and because the ticket\n" +
			"list carries no status or assignee filter they are the only way to express\n" +
			"one. --category separates Gorgias' built-in queues (system) from those the\n" +
			"helpdesk defined itself (user). The view id feeds either `view items` or\n" +
			"`ticket list --view`.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			page.apply(q)
			if category != "" {
				q.Set("category", category)
			}
			resp, err := s.call(cmd.Context(), token, base, http.MethodGet, "/views", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	page.register(cmd)
	cmd.Flags().StringVar(&category, "category", "", "filter by category: system|user")
	return cmd
}

func (s *Service) newViewItemsCmd(token, base string) *cobra.Command {
	var page pageFlags
	cmd := &cobra.Command{
		Use:   "items <view-id>",
		Short: "List a view's items/tickets (GET /views/{id}/items)",
		Long: "The tickets currently sitting in one saved queue — the closest thing to a\n" +
			"filtered ticket search this API offers. The same queue is also reachable\n" +
			"as `ticket list --view <id>`, which returns ticket objects through the\n" +
			"ticket endpoint. Cursor-paged: continue with --cursor.",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			page.apply(q)
			resp, err := s.call(cmd.Context(), token, base, http.MethodGet,
				"/views/"+url.PathEscape(args[0])+"/items", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	page.register(cmd)
	return cmd
}
