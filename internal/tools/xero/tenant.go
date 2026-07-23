package xero

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// guidRe matches a Xero tenantId (a UUID). A --tenant value that is already a
// GUID is used directly, skipping the /connections round trip.
var guidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// connection is one entry from GET /connections: a tenant (organisation) the
// current token can act on.
type connection struct {
	ID         string `json:"id"`
	TenantID   string `json:"tenantId"`
	TenantName string `json:"tenantName"`
	TenantType string `json:"tenantType"`
}

// listConnections fetches the tenants the token can act on. Errors from the
// connections call propagate as apiError (exit 1).
func (s *Service) listConnections(ctx context.Context, token string) ([]connection, []byte, error) {
	body, err := s.call(ctx, token, http.MethodGet, connectionsPath, "", nil, nil)
	if err != nil {
		return nil, nil, err
	}
	var conns []connection
	if err := json.Unmarshal(body, &conns); err != nil {
		return nil, nil, &apiError{msg: fmt.Sprintf("xero: decode /connections: %v", err), err: err}
	}
	return conns, body, nil
}

// resolveTenant maps a caller's tenant selector to a concrete tenantId to send
// in Xero-Tenant-Id. selector is the --tenant flag (or the injected default).
//
//   - selector is a GUID  → used directly (no /connections call).
//   - selector is a name  → matched case-insensitively against /connections;
//     no match or an ambiguous match is a usageError (exit 2).
//   - selector empty       → /connections must resolve to exactly one tenant;
//     >1 is a usageError (exit 2) listing candidates; 0 is an apiError (exit 1).
func (s *Service) resolveTenant(ctx context.Context, token, selector string) (string, error) {
	selector = strings.TrimSpace(selector)
	if guidRe.MatchString(selector) {
		return selector, nil
	}

	conns, _, err := s.listConnections(ctx, token)
	if err != nil {
		return "", err
	}

	if selector != "" {
		var matches []connection
		for _, c := range conns {
			if strings.EqualFold(strings.TrimSpace(c.TenantName), selector) {
				matches = append(matches, c)
			}
		}
		switch len(matches) {
		case 1:
			return matches[0].TenantID, nil
		case 0:
			return "", &usageError{msg: fmt.Sprintf(
				"no Xero organisation named %q on this login; connected organisations: %s",
				selector, candidateList(conns))}
		default:
			return "", &usageError{msg: fmt.Sprintf(
				"organisation name %q is ambiguous; pass --tenant <tenantId>. Matches: %s",
				selector, candidateList(matches))}
		}
	}

	switch len(conns) {
	case 1:
		return conns[0].TenantID, nil
	case 0:
		return "", &apiError{msg: "no Xero organisation connected to this login"}
	default:
		return "", &usageError{msg: fmt.Sprintf(
			"multiple Xero organisations connected; pass --tenant <id|name>. Connected: %s",
			candidateList(conns))}
	}
}

// candidateList renders "name (tenantId)" pairs for a recovery hint.
func candidateList(conns []connection) string {
	parts := make([]string, 0, len(conns))
	for _, c := range conns {
		parts = append(parts, fmt.Sprintf("%s (%s)", c.TenantName, c.TenantID))
	}
	return strings.Join(parts, ", ")
}
