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
		Long: "--email is an EXACT match, not a substring search, and is the way to turn\n" +
			"an address into the numeric id `requester get` takes. --per-page is 1-100,\n" +
			"default 30; --page is 1-based and driven by `next_page`. Requesters are a\n" +
			"separate directory from agents: an employee who also works tickets appears\n" +
			"in both, under different ids.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "Takes the numeric requester id, not an email address — resolve one with\n" +
			"`requester list --email <address>`. Requester and agent ids are not\n" +
			"interchangeable, so an id from `agent list` does not resolve here.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
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
		Long: "Freshservice exposes no /agents/me, so establishing who the connected key\n" +
			"acts as means looking up its own address with --email, which matches\n" +
			"exactly. --per-page is 1-100, default 30; --page is 1-based. The `id` here\n" +
			"is what --agent-id (responder_id) takes on `ticket create` and `ticket\n" +
			"update`, and what an `agent_id:` clause in `ticket search` compares.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "Takes the numeric agent id from `agent list`; this endpoint has no lookup\n" +
			"by email or name. It returns the agent's own profile record, not the\n" +
			"tickets assigned to them — those come from `ticket search --query\n" +
			"\"agent_id:<id>\"`.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
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
		Long: "Assignment groups only. The ids here are what --group-id takes on `ticket\n" +
			"create` and `ticket update`, and what a `group_id:` clause in `ticket\n" +
			"search` compares against. There is no group get and no member expansion in\n" +
			"this tool, so an agent-to-group mapping has to be read off the agent's own\n" +
			"record. --per-page is 1-100, default 30; --page is 1-based.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
