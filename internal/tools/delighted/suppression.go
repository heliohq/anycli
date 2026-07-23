package delighted

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newBouncesCmd wires `delighted bounces list` — GET /bounces.json, people whose
// survey email bounced.
func (s *Service) newBouncesCmd(key string) *cobra.Command {
	cmd := &cobra.Command{Use: "bounces", Short: "Bounced survey recipients"}
	list := &cobra.Command{
		Use:   "list",
		Short: "List bounced people (GET /bounces.json)",
		Args:  cobra.NoArgs,
	}
	list.Annotations = readOnly
	perPage, page := registerPaging(list)
	list.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		applyPaging(q, *perPage, *page)
		resp, err := s.call(cmd.Context(), key, http.MethodGet, "/bounces.json", q, nil)
		if err != nil {
			return err
		}
		return s.emit(resp)
	}
	cmd.AddCommand(list)
	return cmd
}

// newUnsubscribesCmd wires the unsubscribe resource: list existing unsubscribes
// and add a new one, over /unsubscribes(.json).
func (s *Service) newUnsubscribesCmd(key string) *cobra.Command {
	cmd := &cobra.Command{Use: "unsubscribes", Short: "Unsubscribed recipients"}
	cmd.AddCommand(
		s.newUnsubscribesListCmd(key),
		s.newUnsubscribesAddCmd(key),
	)
	return cmd
}

func (s *Service) newUnsubscribesListCmd(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List unsubscribed people (GET /unsubscribes.json)",
		Args:  cobra.NoArgs,
	}
	cmd.Annotations = readOnly
	perPage, page := registerPaging(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		applyPaging(q, *perPage, *page)
		resp, err := s.call(cmd.Context(), key, http.MethodGet, "/unsubscribes.json", q, nil)
		if err != nil {
			return err
		}
		return s.emit(resp)
	}
	return cmd
}

func (s *Service) newUnsubscribesAddCmd(key string) *cobra.Command {
	var personEmail string
	cmd := &cobra.Command{
		Use:         "add",
		Short:       "Unsubscribe a person (POST /unsubscribes.json)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload := map[string]any{"person_email": personEmail}
			resp, err := s.call(cmd.Context(), key, http.MethodPost, "/unsubscribes.json", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&personEmail, "person-email", "", "email of the person to unsubscribe")
	_ = cmd.MarkFlagRequired("person-email")
	return cmd
}
