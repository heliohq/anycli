package freshbooks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// meResponse is the subset of GET /auth/api/v1/users/me the tool consumes: the
// identity fields and the business memberships that carry the account_id every
// accounting URL needs.
type meResponse struct {
	Response struct {
		ID                  json.Number `json:"id"`
		FirstName           string      `json:"first_name"`
		LastName            string      `json:"last_name"`
		Email               string      `json:"email"`
		BusinessMemberships []struct {
			Business struct {
				ID        json.Number `json:"id"`
				Name      string      `json:"name"`
				AccountID string      `json:"account_id"`
			} `json:"business"`
		} `json:"business_memberships"`
	} `json:"response"`
}

// fetchMe GETs the identity endpoint and decodes it.
func (s *Service) fetchMe(ctx context.Context, token string) (*meResponse, error) {
	body, err := s.call(ctx, token, http.MethodGet, "/auth/api/v1/users/me", nil)
	if err != nil {
		return nil, err
	}
	var me meResponse
	if err := json.Unmarshal(body, &me); err != nil {
		return nil, &apiError{msg: fmt.Sprintf("freshbooks: decode identity: %v", err), err: err}
	}
	return &me, nil
}

// accountIDs returns the distinct non-empty account_ids from the memberships,
// preserving order.
func (m *meResponse) accountIDs() []string {
	seen := make(map[string]struct{})
	var out []string
	for _, bm := range m.Response.BusinessMemberships {
		id := strings.TrimSpace(bm.Business.AccountID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// resolveAccount returns the account_id to use for an accounting command. A
// supplied --account short-circuits the identity call. Otherwise it reads
// business_memberships: exactly one account resolves silently; multiple fail
// fast (exit 2) with the available ids and --account guidance; zero fails
// (exit 1) with an explicit "no accounting account" error. It never guesses.
func (s *Service) resolveAccount(ctx context.Context, token, accountFlag string) (string, error) {
	if a := strings.TrimSpace(accountFlag); a != "" {
		return a, nil
	}
	me, err := s.fetchMe(ctx, token)
	if err != nil {
		return "", err
	}
	ids := me.accountIDs()
	switch len(ids) {
	case 1:
		return ids[0], nil
	case 0:
		return "", &apiError{msg: "freshbooks: this FreshBooks identity has no accounting account; connect an account with a business or pass --account"}
	default:
		return "", &usageError{msg: fmt.Sprintf(
			"freshbooks: this identity has %d accounting accounts (%s); pass --account <accountId> to choose one",
			len(ids), strings.Join(ids, ", "))}
	}
}
