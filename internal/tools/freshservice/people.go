package freshservice

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func (s *Service) newRequesterCmd(c *client) *cobra.Command {
	cmd := newResourceGroup("requester", "Requesters (the employees who raise tickets): list, get")
	cmd.AddCommand(
		s.newRequesterListCmd(c),
		s.newRequesterGetCmd(c),
	)
	return cmd
}

// newRequesterListCmd → GET /requesters, optionally filtered by email.
func (s *Service) newRequesterListCmd(c *client) *cobra.Command {
	var email string
	var perPage, page int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List requesters (GET /requesters)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validatePerPage(perPage); err != nil {
				return err
			}
			q := url.Values{}
			q.Set("page", strconv.Itoa(page))
			q.Set("per_page", strconv.Itoa(perPage))
			if email != "" {
				q.Set("email", email)
			}
			return s.emitListResult(cmd, c, "/requesters", "requesters", q, page, perPage)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "filter by exact email")
	cmd.Flags().IntVar(&perPage, "per-page", defaultPerPage, "results per page (max 100)")
	cmd.Flags().IntVar(&page, "page", 1, "1-based page number")
	return cmd
}

func (s *Service) newRequesterGetCmd(c *client) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get one requester (GET /requesters/{id})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, _, err := c.call(cmd.Context(), http.MethodGet, "/requesters/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emitResource(body, "requester")
		},
	}
}

func (s *Service) newAgentCmd(c *client) *cobra.Command {
	cmd := newResourceGroup("agent", "Agents (assignment targets + operator self-context): list, get")
	cmd.AddCommand(
		s.newAgentListCmd(c),
		s.newAgentGetCmd(c),
	)
	return cmd
}

// newAgentListCmd → GET /agents. Freshservice has no /agents/me, so operator
// self-context is an ?email= lookup.
func (s *Service) newAgentListCmd(c *client) *cobra.Command {
	var email string
	var perPage, page int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List agents (GET /agents)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validatePerPage(perPage); err != nil {
				return err
			}
			q := url.Values{}
			q.Set("page", strconv.Itoa(page))
			q.Set("per_page", strconv.Itoa(perPage))
			if email != "" {
				q.Set("email", email)
			}
			return s.emitListResult(cmd, c, "/agents", "agents", q, page, perPage)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "filter by exact email (there is no /agents/me)")
	cmd.Flags().IntVar(&perPage, "per-page", defaultPerPage, "results per page (max 100)")
	cmd.Flags().IntVar(&page, "page", 1, "1-based page number")
	return cmd
}

func (s *Service) newAgentGetCmd(c *client) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get one agent (GET /agents/{id})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, _, err := c.call(cmd.Context(), http.MethodGet, "/agents/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emitResource(body, "agent")
		},
	}
}

func (s *Service) newGroupCmd(c *client) *cobra.Command {
	cmd := newResourceGroup("group", "Assignment groups: list")
	cmd.AddCommand(s.newGroupListCmd(c))
	return cmd
}

func (s *Service) newGroupListCmd(c *client) *cobra.Command {
	var perPage, page int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List groups (GET /groups)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validatePerPage(perPage); err != nil {
				return err
			}
			q := url.Values{}
			q.Set("page", strconv.Itoa(page))
			q.Set("per_page", strconv.Itoa(perPage))
			return s.emitListResult(cmd, c, "/groups", "groups", q, page, perPage)
		},
	}
	cmd.Flags().IntVar(&perPage, "per-page", defaultPerPage, "results per page (max 100)")
	cmd.Flags().IntVar(&page, "page", 1, "1-based page number")
	return cmd
}
