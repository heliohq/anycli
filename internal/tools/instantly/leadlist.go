package instantly

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newLeadListCmd(token string) *cobra.Command {
	cmd := newGroupCmd("lead-list", "Lead lists (staging leads before campaign assignment)")
	cmd.AddCommand(
		s.newLeadListListCmd(token),
		s.newLeadListGetCmd(token),
		s.newLeadListCreateCmd(token),
		s.newLeadListUpdateCmd(token),
		s.newLeadListDeleteCmd(token),
		s.newLeadListVerificationStatsCmd(token),
	)
	return cmd
}

func (s *Service) newLeadListListCmd(token string) *cobra.Command {
	var page pageFlags
	var search string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List lead lists (GET /lead-lists)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			page.applyQuery(q)
			setIfChanged(cmd, q, "search", "search", search)
			return s.get(cmd, token, "/lead-lists", q)
		},
	}
	registerPageFlags(cmd, &page)
	cmd.Flags().StringVar(&search, "search", "", "filter by name substring")
	return cmd
}

func (s *Service) newLeadListGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a lead list (GET /lead-lists/{id})",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.get(cmd, token, "/lead-lists/"+url.PathEscape(id), nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "lead-list id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newLeadListCreateCmd(token string) *cobra.Command {
	var data, name string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a lead list (POST /lead-lists)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := decodeDataFlag(data)
			if err != nil {
				return err
			}
			setBodyIfChanged(cmd, body, "name", "name", name)
			return s.send(cmd, token, http.MethodPost, "/lead-lists", body)
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "raw JSON lead-list body (merged; flags override)")
	cmd.Flags().StringVar(&name, "name", "", "lead-list name")
	return cmd
}

func (s *Service) newLeadListUpdateCmd(token string) *cobra.Command {
	var id, data string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a lead list (PATCH /lead-lists/{id}). --data is the raw JSON body",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := decodeDataFlag(data)
			if err != nil {
				return err
			}
			return s.send(cmd, token, http.MethodPatch, "/lead-lists/"+url.PathEscape(id), body)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "lead-list id")
	cmd.Flags().StringVar(&data, "data", "", "raw JSON patch body")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newLeadListDeleteCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a lead list (DELETE /lead-lists/{id})",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.send(cmd, token, http.MethodDelete, "/lead-lists/"+url.PathEscape(id), nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "lead-list id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newLeadListVerificationStatsCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "verification-stats",
		Short: "Email-verification stats for a lead list (GET /lead-lists/{id}/verification-stats)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.get(cmd, token, "/lead-lists/"+url.PathEscape(id)+"/verification-stats", nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "lead-list id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}
