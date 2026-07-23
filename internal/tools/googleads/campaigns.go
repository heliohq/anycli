package googleads

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
)

// campaignListStatuses is the closed set of campaign.status values accepted as
// a --status filter on `campaigns list`.
var campaignListStatuses = map[string]struct{}{"ENABLED": {}, "PAUSED": {}, "REMOVED": {}}

// campaignSetStatuses is the closed set accepted by `campaign set-status`.
// REMOVED is intentionally excluded: v1 steers, it does not delete.
var campaignSetStatuses = map[string]struct{}{"ENABLED": {}, "PAUSED": {}}

// newCampaignsListCmd lists campaigns with key metrics via a composed GAQL
// SELECT over the paged googleAds:search. An optional --status filters the WHERE
// clause. Emits the search response verbatim (results + nextPageToken).
func (s *Service) newCampaignsListCmd(c creds) *cobra.Command {
	var customerID, status string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List campaigns with key metrics (POST googleAds:search)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			id, err := normalizeCustomerID(customerID)
			if err != nil {
				return err
			}
			gaql := "SELECT campaign.id, campaign.name, campaign.status, campaign.advertising_channel_type, " +
				"metrics.impressions, metrics.clicks, metrics.cost_micros FROM campaign"
			if trimmed := strings.ToUpper(strings.TrimSpace(status)); trimmed != "" {
				if _, ok := campaignListStatuses[trimmed]; !ok {
					return &usageError{msg: fmt.Sprintf("--status %q is invalid (want ENABLED|PAUSED|REMOVED)", status)}
				}
				gaql += " WHERE campaign.status = '" + trimmed + "'"
			}
			return s.runSearch(cmd, c, id, gaql, false, 0, "")
		},
	}
	cmd.Flags().StringVar(&customerID, "customer-id", "", "target Google Ads customer id (required)")
	cmd.Flags().StringVar(&status, "status", "", "optional status filter: ENABLED|PAUSED|REMOVED")
	_ = cmd.MarkFlagRequired("customer-id")
	return cmd
}

// newCampaignSetStatusCmd flips one campaign's status (ENABLED|PAUSED) via
// campaigns:mutate with an explicit updateMask. Explicit-id only; no create /
// delete. Emits the mutate response (mutated resource name) verbatim.
func (s *Service) newCampaignSetStatusCmd(c creds) *cobra.Command {
	var customerID, campaignID, status string
	cmd := &cobra.Command{
		Use:   "set-status",
		Short: "Enable or pause a campaign (POST campaigns:mutate)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			id, err := normalizeCustomerID(customerID)
			if err != nil {
				return err
			}
			if strings.TrimSpace(campaignID) == "" {
				return &usageError{msg: "--id is required"}
			}
			up := strings.ToUpper(strings.TrimSpace(status))
			if _, ok := campaignSetStatuses[up]; !ok {
				return &usageError{msg: fmt.Sprintf("--status %q is invalid (want ENABLED|PAUSED)", status)}
			}
			lc, err := loginCustomerID(cmd)
			if err != nil {
				return err
			}
			c.loginCustomerID = lc
			payload := map[string]any{
				"operations": []any{
					map[string]any{
						"updateMask": "status",
						"update": map[string]any{
							"resourceName": fmt.Sprintf("customers/%s/campaigns/%s", id, strings.TrimSpace(campaignID)),
							"status":       up,
						},
					},
				},
			}
			body, err := s.call(cmd.Context(), c, http.MethodPost, "/customers/"+id+"/campaigns:mutate", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&customerID, "customer-id", "", "target Google Ads customer id (required)")
	cmd.Flags().StringVar(&campaignID, "id", "", "campaign id (required)")
	cmd.Flags().StringVar(&status, "status", "", "ENABLED or PAUSED (required)")
	_ = cmd.MarkFlagRequired("customer-id")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("status")
	return cmd
}

// newBudgetSetCmd sets a campaign budget's daily amount (in micros) via
// campaignBudgets:mutate. Explicit-id only. amountMicros is sent as a string
// (Google encodes int64 fields as JSON strings). Emits the response verbatim.
func (s *Service) newBudgetSetCmd(c creds) *cobra.Command {
	var customerID, budgetID string
	var amountMicros int64
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set a campaign budget's amount in micros (POST campaignBudgets:mutate)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			id, err := normalizeCustomerID(customerID)
			if err != nil {
				return err
			}
			if strings.TrimSpace(budgetID) == "" {
				return &usageError{msg: "--id is required"}
			}
			if amountMicros <= 0 {
				return &usageError{msg: "--amount-micros must be a positive integer (1_000_000 micros = 1 unit of the account currency)"}
			}
			lc, err := loginCustomerID(cmd)
			if err != nil {
				return err
			}
			c.loginCustomerID = lc
			payload := map[string]any{
				"operations": []any{
					map[string]any{
						"updateMask": "amount_micros",
						"update": map[string]any{
							"resourceName": fmt.Sprintf("customers/%s/campaignBudgets/%s", id, strings.TrimSpace(budgetID)),
							"amountMicros": intToString(amountMicros),
						},
					},
				},
			}
			body, err := s.call(cmd.Context(), c, http.MethodPost, "/customers/"+id+"/campaignBudgets:mutate", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&customerID, "customer-id", "", "target Google Ads customer id (required)")
	cmd.Flags().StringVar(&budgetID, "id", "", "campaign budget id (required)")
	cmd.Flags().Int64Var(&amountMicros, "amount-micros", 0, "new daily budget amount in micros (required, positive)")
	_ = cmd.MarkFlagRequired("customer-id")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("amount-micros")
	return cmd
}
