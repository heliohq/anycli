package salesloft

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newActivityCmd exposes the unified recent-engagement feed
// (GET /v2/activity_histories).
func (s *Service) newActivityCmd(token string) *cobra.Command {
	cmd := newGroupCmd("activity", "Read the activity history feed")
	cmd.AddCommand(s.newActivityListCmd(token))
	return cmd
}

func (s *Service) newActivityListCmd(token string) *cobra.Command {
	var lf listFlags
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List activity history (GET /v2/activity_histories)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.listInto(cmd, token, "/activity_histories", &lf)
		},
	}
	registerListFlags(cmd, &lf)
	return cmd
}

// newEmailCmd groups the read-only outreach-email activity views.
func (s *Service) newEmailCmd(token string) *cobra.Command {
	cmd := newGroupCmd("email", "Read email activity")
	cmd.AddCommand(
		s.newEmailListCmd(token),
		s.newEmailGetCmd(token),
	)
	return cmd
}

func (s *Service) newEmailListCmd(token string) *cobra.Command {
	var lf listFlags
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List email activities (GET /v2/activities/emails)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.listInto(cmd, token, "/activities/emails", &lf)
		},
	}
	registerListFlags(cmd, &lf)
	return cmd
}

func (s *Service) newEmailGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Fetch one email activity (GET /v2/activities/emails/{id})",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/activities/emails/"+id, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "email activity id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

// newCallCmd exposes the read-only call activity list.
func (s *Service) newCallCmd(token string) *cobra.Command {
	cmd := newGroupCmd("call", "Read call activity")
	cmd.AddCommand(s.newCallListCmd(token))
	return cmd
}

func (s *Service) newCallListCmd(token string) *cobra.Command {
	var lf listFlags
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List call activities (GET /v2/activities/calls)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.listInto(cmd, token, "/activities/calls", &lf)
		},
	}
	registerListFlags(cmd, &lf)
	return cmd
}

// listInto runs a plain GET list with only the shared list flags and emits the
// passthrough envelope — shared by the activity/email/call feed lists.
func (s *Service) listInto(cmd *cobra.Command, token, path string, lf *listFlags) error {
	q, err := lf.values()
	if err != nil {
		return err
	}
	resp, err := s.call(cmd.Context(), token, http.MethodGet, path, q, nil)
	if err != nil {
		return err
	}
	return s.emit(resp)
}
