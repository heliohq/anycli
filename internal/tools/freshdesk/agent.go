package freshdesk

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newAgentCmd(c *client) *cobra.Command {
	cmd := &cobra.Command{Use: "agent", Short: "Agents (list, get, me)"}
	cmd.AddCommand(
		s.newAgentListCmd(c),
		s.newAgentGetCmd(c),
		s.newAgentMeCmd(c),
	)
	return cmd
}

func (s *Service) newAgentListCmd(c *client) *cobra.Command {
	var email string
	var page, perPage int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List agents (GET /agents)",
		Long: "Agents are the helpdesk's own staff, a separate population from contacts,\n" +
			"and their ids are not contact ids. Each row's `id` is what\n" +
			"--responder-id takes on `ticket create` and `ticket update`. --email is an\n" +
			"exact match, so it answers whether one person is an agent in a single\n" +
			"call.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setNonEmpty(q, "email", email)
			applyPaging(q, page, perPage)
			resp, err := c.call(cmd.Context(), http.MethodGet, "/agents", q, nil)
			if err != nil {
				return err
			}
			return c.emit(resp)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "filter by agent email")
	registerPagingFlags(cmd, &page, &perPage)
	return cmd
}

func (s *Service) newAgentGetCmd(c *client) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get an agent (GET /agents/{id})",
		Long: "Takes the numeric agent id. Name and email sit inside the nested `contact`\n" +
			"object, not at the top level, which is where the top-level fields — role,\n" +
			"ticket scope, group membership — differ from a contact record of the same\n" +
			"person.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := c.call(cmd.Context(), http.MethodGet, "/agents/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			return c.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "agent id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newAgentMeCmd(c *client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "me",
		Short: "Get the currently authenticated agent (GET /agents/me) — the connectivity/identity check",
		Long: "Resolves the API key to the agent it belongs to, which makes it the cheapest\n" +
			"check that both halves of the credential are right: a wrong subdomain or a\n" +
			"key that was reset in Freshdesk fails here before any ticket is touched.\n" +
			"The returned id is this account's own --responder-id, for\n" +
			"self-assignment.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := c.call(cmd.Context(), http.MethodGet, "/agents/me", nil, nil)
			if err != nil {
				return err
			}
			return c.emit(resp)
		},
	}
	return cmd
}
