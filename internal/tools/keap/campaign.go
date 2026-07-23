package keap

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// Campaigns are the legacy (Campaign Builder) automation surface, read-only in
// this tool.
func (s *Service) newCampaignCmd(token string) *cobra.Command {
	cmd := newGroupCmd("campaign", "Legacy campaigns (list, get)")
	cmd.AddCommand(
		s.newCampaignListCmd(token),
		s.newCampaignGetCmd(token),
	)
	return cmd
}

func (s *Service) newCampaignListCmd(token string) *cobra.Command {
	var lf *listFlags
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List campaigns (GET /v2/campaigns)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/campaigns", lf.values(), nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	lf = registerListFlags(cmd)
	return cmd
}

func (s *Service) newCampaignGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "get <campaign-id>",
		Short:       "Get a campaign (GET /v2/campaigns/{id})",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/campaigns/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}
