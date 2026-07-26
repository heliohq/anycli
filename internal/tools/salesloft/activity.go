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
		Use:   "list",
		Short: "List activity history (GET /v2/activity_histories)",
		Long: "One stream carrying every kind of logged engagement — emails, calls,\n" +
			"notes and the rest — which makes it the right first read for \"what has\n" +
			"happened lately\" before drilling into `email list` or `call list`. It\n" +
			"is a team-wide firehose, so pair `--updated-since` with `--filter`\n" +
			"rather than paging it: deep pages cost extra against the shared rate\n" +
			"budget. Entries name the record they belong to, not just the actor.",
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
		Use:   "list",
		Short: "List email activities (GET /v2/activities/emails)",
		Long: "Outreach emails as activity records: each carries the send status and\n" +
			"the engagement counters — opens, clicks, replies — rather than the\n" +
			"message body. Narrow with `--filter` on Salesloft's documented\n" +
			"email-activity filters and read incrementally with `--updated-since`.\n" +
			"Read-only: nothing in this tool sends an email, which happens through a\n" +
			"cadence enrollment instead.",
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
		Use:   "get",
		Short: "Fetch one email activity (GET /v2/activities/emails/{id})",
		Long: "`--id` is required and is an email ACTIVITY id from `email list` — not a\n" +
			"person, cadence or task id. Returns that one email with its subject,\n" +
			"status, timestamps, the person it went to and its engagement counters.",
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
		Use:   "list",
		Short: "List call activities (GET /v2/activities/calls)",
		Long: "Calls Salesloft has already recorded, with their disposition,\n" +
			"sentiment, duration and the person and user each is attached to. There\n" +
			"is no `call get`, so a single call is reached by narrowing this list\n" +
			"with `--filter` and `--updated-since`. Read-only: nothing here places\n" +
			"or logs a call.",
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
