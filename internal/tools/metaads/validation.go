package metaads

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	accountIDPattern = regexp.MustCompile(`^act_[0-9]{1,20}$`)
	objectIDPattern  = regexp.MustCompile(`^[0-9]{1,25}$`)
)

// requireAccountID validates an ad-account identifier in its canonical
// act_<numeric> form (the form /me/adaccounts returns and every act_ edge
// requires). A bare numeric id is rejected so a caller cannot accidentally
// address a different object type.
func requireAccountID(value string) error {
	if !accountIDPattern.MatchString(value) {
		return fmt.Errorf("--account must be an ad account id in act_<number> form (from `accounts list`)")
	}
	return nil
}

// requireObjectID validates a numeric Graph object id (campaign / ad set / ad
// / creative).
func requireObjectID(name, value string) error {
	if !objectIDPattern.MatchString(value) {
		return fmt.Errorf("%s must be a numeric Meta object id", name)
	}
	return nil
}

func requireOptionalObjectID(name, value string) error {
	if value == "" {
		return nil
	}
	return requireObjectID(name, value)
}

// requireInsightsLevel validates the reporting aggregation level.
func requireInsightsLevel(value string) error {
	switch value {
	case "", "account", "campaign", "adset", "ad":
		return nil
	default:
		return fmt.Errorf(`--level must be one of account, campaign, adset, ad`)
	}
}

func requireExactlyOne(firstName, firstValue, secondName, secondValue string) error {
	firstSet := strings.TrimSpace(firstValue) != ""
	secondSet := strings.TrimSpace(secondValue) != ""
	if firstSet == secondSet {
		return fmt.Errorf("exactly one of %s or %s is required", firstName, secondName)
	}
	return nil
}

func requireAtMostOne(firstName, firstValue, secondName, secondValue string) error {
	if strings.TrimSpace(firstValue) != "" && strings.TrimSpace(secondValue) != "" {
		return fmt.Errorf("at most one of %s or %s may be set", firstName, secondName)
	}
	return nil
}

func requireLimit(value, min, max int) error {
	if value < min || value > max {
		return fmt.Errorf("--limit must be between %d and %d", min, max)
	}
	return nil
}
