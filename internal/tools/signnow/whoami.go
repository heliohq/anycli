package signnow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

// rawUser is the subset of GET /user the tool reads: identity plus the primary
// email used as the invite sender default.
type rawUser struct {
	ID           string `json:"id"`
	PrimaryEmail string `json:"primary_email"`
	Email        string `json:"email"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
}

func (s *Service) newWhoamiCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the authenticated SignNow account",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			u, err := s.fetchUser(cmd.Context(), token)
			if err != nil {
				return err
			}
			return s.emitJSON(map[string]any{
				"id":            u.ID,
				"primary_email": u.primaryEmail(),
				"first_name":    u.FirstName,
				"last_name":     u.LastName,
			})
		},
	}
}

func (s *Service) fetchUser(ctx context.Context, token string) (rawUser, error) {
	body, err := s.call(ctx, token, http.MethodGet, "/user", nil, nil)
	if err != nil {
		return rawUser{}, err
	}
	var u rawUser
	if err := json.Unmarshal(body, &u); err != nil {
		return rawUser{}, &apiError{msg: fmt.Sprintf("signnow: decode user: %v", err), err: err}
	}
	return u, nil
}

// primaryEmail prefers the primary_email field, falling back to email.
func (u rawUser) primaryEmail() string {
	if u.PrimaryEmail != "" {
		return u.PrimaryEmail
	}
	return u.Email
}
