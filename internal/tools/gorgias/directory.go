package gorgias

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// This file holds the read-only directory/reporting resources: agents (users),
// tags, views, satisfaction surveys, and the account identity anchor.

func (s *Service) newUserCmd(token, base string) *cobra.Command {
	cmd := newGroupCmd("user", "Resolve agents (list, get)")
	cmd.AddCommand(
		s.newSimpleListCmd(token, base, "list", "List agents (GET /users)", "/users"),
		s.newSimpleGetCmd(token, base, "get <user-id>", "Retrieve an agent (GET /users/{id})", "/users/"),
	)
	return cmd
}

func (s *Service) newTagCmd(token, base string) *cobra.Command {
	cmd := newGroupCmd("tag", "Tag lookup for triage")
	cmd.AddCommand(
		s.newSimpleListCmd(token, base, "list", "List tags (GET /tags)", "/tags"),
	)
	return cmd
}

func (s *Service) newSatisfactionCmd(token, base string) *cobra.Command {
	cmd := newGroupCmd("satisfaction", "CSAT survey reporting")
	cmd.AddCommand(
		s.newSimpleListCmd(token, base, "list", "List satisfaction surveys (GET /satisfaction-surveys)", "/satisfaction-surveys"),
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
		Args:  cobra.NoArgs,
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
func (s *Service) newSimpleListCmd(token, base, use, short, path string) *cobra.Command {
	var page pageFlags
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
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
func (s *Service) newSimpleGetCmd(token, base, use, short, pathPrefix string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(1),
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
		Args:  cobra.NoArgs,
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
		Args:  cobra.ExactArgs(1),
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
