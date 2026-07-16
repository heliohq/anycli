package drive

import (
	"fmt"
	"strings"
)

// cleanIDs splits every multi-id arg on whitespace and drops empties. Ids from
// pipelines carry trailing \r or several ids pasted into one arg; Drive ids
// never contain whitespace, so Fields-splitting is always safe.
func cleanIDs(args []string) ([]string, error) {
	ids := make([]string, 0, len(args))
	for _, arg := range args {
		ids = append(ids, strings.Fields(arg)...)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("drive: no valid file ids")
	}
	return ids, nil
}

// orDash renders an empty string as a dash for human output.
func orDash(v string) string {
	if strings.TrimSpace(v) == "" {
		return "-"
	}
	return v
}

// quotaLimit renders the storage limit; an empty/zero limit means "unlimited"
// (Workspace pooled/unlimited quotas omit the field).
func quotaLimit(limit string) string {
	if strings.TrimSpace(limit) == "" || limit == "0" {
		return "unlimited"
	}
	return limit
}

// ownerLabel renders the first owner as "Name <email>".
func ownerLabel(owners []struct {
	DisplayName  string `json:"displayName"`
	EmailAddress string `json:"emailAddress"`
}) string {
	if len(owners) == 0 {
		return "-"
	}
	o := owners[0]
	if o.EmailAddress == "" {
		return orDash(o.DisplayName)
	}
	return fmt.Sprintf("%s <%s>", o.DisplayName, o.EmailAddress)
}

// permTarget renders a permission's subject: the email, else the domain, else
// "anyone".
func permTarget(email, domain string) string {
	switch {
	case email != "":
		return email
	case domain != "":
		return domain
	default:
		return "anyone"
	}
}
