package salesloft

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newCadenceCmd groups cadence discovery, enrollment (the core write), and
// membership-state lookup.
func (s *Service) newCadenceCmd(token string) *cobra.Command {
	cmd := newGroupCmd("cadence", "Manage cadences and enrollment")
	cmd.AddCommand(
		s.newCadenceListCmd(token),
		s.newCadenceGetCmd(token),
		s.newCadenceAddPersonCmd(token),
		s.newCadenceMembershipsCmd(token),
	)
	return cmd
}

func (s *Service) newCadenceListCmd(token string) *cobra.Command {
	var lf listFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List cadences (GET /v2/cadences)",
		Long: "Cadences are the outreach sequences a person can be enrolled into, and\n" +
			"this is where enrollment gets its `--cadence-id`. Each entry says who\n" +
			"owns the cadence, whether it is shared with the team and how many steps\n" +
			"it has — worth reading before enrolling anyone, because the step count\n" +
			"and type decide how much outreach one enrollment sets in motion.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q, err := lf.values()
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/cadences", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerListFlags(cmd, &lf)
	return cmd
}

func (s *Service) newCadenceGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Fetch one cadence (GET /v2/cadences/{id})",
		Long: "`--id` is required. Returns the cadence's settings and counters — owner,\n" +
			"team visibility, step count, and how many people are currently in it —\n" +
			"which is what tells you whether an enrollment starts sending email\n" +
			"straight away or only creates a task for a rep. The individual steps\n" +
			"themselves are not reachable through this tool.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/cadences/"+id, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "cadence id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newCadenceAddPersonCmd(token string) *cobra.Command {
	var personID, cadenceID, userID int
	cmd := &cobra.Command{
		Use:   "add-person",
		Short: "Enroll a person into a cadence (POST /v2/cadence_memberships)",
		Long: "The one enrollment write, and the point where this tool starts real\n" +
			"outreach: `--person-id` and `--cadence-id` are both required and the\n" +
			"person enters the cadence immediately, so Salesloft may send the first\n" +
			"step or put it on a rep's queue without any further step from here.\n" +
			"`--user-id` defaults to the authenticated user; setting it enrolls on\n" +
			"another rep's behalf and attributes the outreach to them. There is no\n" +
			"unenroll command — stopping a mistaken enrollment happens in Salesloft.\n" +
			"Check `cadence memberships --person-id --cadence-id` first rather than\n" +
			"enrolling someone who is already in it.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{
				"person_id":  personID,
				"cadence_id": cadenceID,
			}
			// user_id defaults to the authenticated user; only send it when set.
			if cmd.Flags().Changed("user-id") {
				body["user_id"] = userID
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/cadence_memberships", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&personID, "person-id", 0, "person id to enroll")
	cmd.Flags().IntVar(&cadenceID, "cadence-id", 0, "cadence id to enroll into")
	cmd.Flags().IntVar(&userID, "user-id", 0, "assigned user id (defaults to the authenticated user)")
	_ = cmd.MarkFlagRequired("person-id")
	_ = cmd.MarkFlagRequired("cadence-id")
	return cmd
}

func (s *Service) newCadenceMembershipsCmd(token string) *cobra.Command {
	var lf listFlags
	var personIDs, cadenceIDs []string
	cmd := &cobra.Command{
		Use:   "memberships",
		Short: "List cadence memberships (GET /v2/cadence_memberships); filter by --person-id / --cadence-id",
		Long: "The state view behind `cadence add-person`: who is in which cadence and\n" +
			"how far through. Both `--person-id` and `--cadence-id` are repeatable\n" +
			"FILTERS, not the id of a membership, so passing one of each answers\n" +
			"\"is this person already in this cadence\" in a single call. Each\n" +
			"membership carries its current step, status and the counters for what\n" +
			"has been sent, which is how you tell an active enrollment from one that\n" +
			"has already run its course.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q, err := lf.values()
			if err != nil {
				return err
			}
			for _, p := range personIDs {
				q.Add("person_id[]", p)
			}
			for _, c := range cadenceIDs {
				q.Add("cadence_id[]", c)
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/cadence_memberships", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerListFlags(cmd, &lf)
	cmd.Flags().StringArrayVar(&personIDs, "person-id", nil, "filter by person id (repeatable)")
	cmd.Flags().StringArrayVar(&cadenceIDs, "cadence-id", nil, "filter by cadence id (repeatable)")
	return cmd
}
