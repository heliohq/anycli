package buffer

import (
	"context"

	"github.com/spf13/cobra"
)

// accountQuery reads the authenticated account plus its organizations. Fields
// are kept to those the official docs reference (account: id, email;
// organization: id, name) to stay fail-fast against unverified schema fields.
// `email` appears in the auth-guide example but NOT in the data-model example,
// so treat it as pending live-schema confirmation on the mandatory L2 real-API
// harness run before the visible flip. If L2 shows `email` is not a valid
// Account field, drop it here and in the Helio identity query and rely on
// `account.id` (the stable identity key). `id` and `organizations` are the
// docs-confirmed fields.
const accountQuery = `query { account { id email organizations { id name } } }`

// organization is one Buffer workspace under the account.
type organization struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// account is the provider-neutral projection of the account query.
type account struct {
	ID            string         `json:"id"`
	Email         string         `json:"email"`
	Organizations []organization `json:"organizations"`
}

func (s *Service) newAccountGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "get",
		Short:       "Get the authenticated account (id, email, organizations)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			acct, err := s.fetchAccount(cmd.Context(), token)
			if err != nil {
				return err
			}
			return s.emitValue(acct)
		},
	}
}

func (s *Service) newOrgListCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "list",
		Short:       "List organizations (workspaces) on the account",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			acct, err := s.fetchAccount(cmd.Context(), token)
			if err != nil {
				return err
			}
			return s.emitValue(map[string]any{"organizations": acct.Organizations})
		},
	}
}

// fetchAccount runs the account query and returns the neutral projection.
func (s *Service) fetchAccount(ctx context.Context, token string) (account, error) {
	data, err := s.gql(ctx, token, accountQuery, nil)
	if err != nil {
		return account{}, err
	}
	var acct account
	if err := decodeField(data, "account", &acct); err != nil {
		return account{}, err
	}
	if acct.Organizations == nil {
		acct.Organizations = []organization{}
	}
	return acct, nil
}
