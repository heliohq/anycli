package metaads

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

func errRequired(flag string) error {
	return fmt.Errorf("%s is required", flag)
}

// updateForm carries the mutable spend-state fields shared by campaign, ad
// set, and ad updates. Budgets are integers in the ad account currency's
// minor unit (cents), matching the Graph API contract.
type updateForm struct {
	status         string
	dailyBudget    int64
	lifetimeBudget int64
}

func (u *updateForm) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&u.status, "status", "", "new status (ACTIVE, PAUSED, ARCHIVED)")
	cmd.Flags().Int64Var(&u.dailyBudget, "daily-budget", 0, "daily budget in the account currency minor unit (cents)")
	cmd.Flags().Int64Var(&u.lifetimeBudget, "lifetime-budget", 0, "lifetime budget in the account currency minor unit (cents)")
}

// run validates the object id, requires at least one mutation, and POSTs the
// update to the object node.
func (u *updateForm) run(s *Service, cmd *cobra.Command, token, name, id, newName string) error {
	if err := requireObjectID(name, id); err != nil {
		return err
	}
	form := url.Values{}
	if u.status != "" {
		if err := requireStatus(u.status); err != nil {
			return err
		}
		form.Set("status", u.status)
	}
	if u.dailyBudget > 0 {
		form.Set("daily_budget", itoa64(u.dailyBudget))
	}
	if u.lifetimeBudget > 0 {
		form.Set("lifetime_budget", itoa64(u.lifetimeBudget))
	}
	if newName != "" {
		form.Set("name", newName)
	}
	if len(form) == 0 {
		return fmt.Errorf("nothing to update: set at least one of --status, --daily-budget, --lifetime-budget, or --name")
	}
	body, err := s.post(cmd.Context(), token, "/"+id, form)
	if err != nil {
		return err
	}
	return s.emit(body)
}

func requireStatus(value string) error {
	switch value {
	case "ACTIVE", "PAUSED", "ARCHIVED":
		return nil
	default:
		return fmt.Errorf("--status must be one of ACTIVE, PAUSED, ARCHIVED")
	}
}
