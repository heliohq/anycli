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
		Use:         "list",
		Short:       "List agents (GET /agents)",
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
		Use:         "get",
		Short:       "Get an agent (GET /agents/{id})",
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
		Use:         "me",
		Short:       "Get the currently authenticated agent (GET /agents/me) — the connectivity/identity check",
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
