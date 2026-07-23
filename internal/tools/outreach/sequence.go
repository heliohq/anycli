package outreach

import (
	"net/url"

	"github.com/spf13/cobra"
)

var (
	sequenceResource     = resource{path: "sequences", typ: "sequence"}
	sequenceStepResource = resource{path: "sequenceSteps", typ: "sequenceStep"}
	enrollmentResource   = resource{path: "sequenceStates", typ: "sequenceState"}
)

// newSequenceCmd builds the sequence resource group: pick the cadence to enroll
// into and inspect its steps.
func (s *Service) newSequenceCmd(token string) *cobra.Command {
	group := newGroupCmd("sequence", "List and inspect sequences (cadences)")
	group.AddCommand(
		s.newSequenceListCmd(token),
		s.newGetCmd(token, sequenceResource),
		s.newSequenceStepsCmd(token),
	)
	return group
}

func (s *Service) newSequenceListCmd(token string) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List sequences (one page)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			query := url.Values{}
			setFilter(query, "name", name)
			if err := listFlagsFrom(cmd).apply(query, sequenceResource.typ); err != nil {
				return err
			}
			return s.runList(cmd.Context(), token, sequenceResource, query)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "filter by sequence name")
	bindListFlags(cmd)
	return cmd
}

// newSequenceStepsCmd lists the sequenceSteps of one sequence (its cadence content).
func (s *Service) newSequenceStepsCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "steps <sequence-id>",
		Short:       "List the steps of one sequence",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireNumericID("sequence id", args[0]); err != nil {
				return err
			}
			query := url.Values{}
			setRelFilter(query, "sequence", args[0])
			if err := listFlagsFrom(cmd).apply(query, sequenceStepResource.typ); err != nil {
				return err
			}
			return s.runList(cmd.Context(), token, sequenceStepResource, query)
		},
	}
	bindListFlags(cmd)
	return cmd
}

// newEnrollmentCmd builds the enrollment group over sequenceStates — the core
// sales-engagement write. "enrollment" is the human word for a sequenceState.
func (s *Service) newEnrollmentCmd(token string) *cobra.Command {
	group := newGroupCmd("enrollment", "Enroll prospects in sequences and manage their sequence state")
	group.AddCommand(
		s.newEnrollmentListCmd(token),
		s.newEnrollmentAddCmd(token),
		s.newEnrollmentActionCmd(token, "pause", "Pause an enrollment"),
		s.newEnrollmentActionCmd(token, "resume", "Resume a paused enrollment"),
		s.newEnrollmentActionCmd(token, "finish", "Finish (stop) an enrollment"),
	)
	return group
}

func (s *Service) newEnrollmentListCmd(token string) *cobra.Command {
	var prospectID, sequenceID, state string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List enrollments / sequence states (one page)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			query := url.Values{}
			setRelFilter(query, "prospect", prospectID)
			setRelFilter(query, "sequence", sequenceID)
			setFilter(query, "state", state)
			if err := listFlagsFrom(cmd).apply(query, enrollmentResource.typ); err != nil {
				return err
			}
			return s.runList(cmd.Context(), token, enrollmentResource, query)
		},
	}
	cmd.Flags().StringVar(&prospectID, "prospect-id", "", "filter by prospect id")
	cmd.Flags().StringVar(&sequenceID, "sequence-id", "", "filter by sequence id")
	cmd.Flags().StringVar(&state, "state", "", "filter by sequence-state value (e.g. active, finished)")
	bindListFlags(cmd)
	return cmd
}

// newEnrollmentAddCmd enrolls a prospect into a sequence via a mailbox by
// creating a sequenceState with the three required relationships.
func (s *Service) newEnrollmentAddCmd(token string) *cobra.Command {
	var prospectID, sequenceID, mailboxID string
	cmd := &cobra.Command{
		Use:         "add",
		Short:       "Enroll a prospect in a sequence (via a mailbox)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			for _, v := range []struct{ label, id string }{
				{"prospect id", prospectID}, {"sequence id", sequenceID}, {"mailbox id", mailboxID},
			} {
				if err := requireNumericID(v.label, v.id); err != nil {
					return err
				}
			}
			rels := map[string]string{"prospect": prospectID, "sequence": sequenceID, "mailbox": mailboxID}
			return s.runCreate(cmd.Context(), token, enrollmentResource, nil, rels)
		},
	}
	cmd.Flags().StringVar(&prospectID, "prospect-id", "", "prospect to enroll (required)")
	cmd.Flags().StringVar(&sequenceID, "sequence-id", "", "sequence to enroll into (required)")
	cmd.Flags().StringVar(&mailboxID, "mailbox-id", "", "mailbox that sends the cadence (required)")
	_ = cmd.MarkFlagRequired("prospect-id")
	_ = cmd.MarkFlagRequired("sequence-id")
	_ = cmd.MarkFlagRequired("mailbox-id")
	return cmd
}

// newEnrollmentActionCmd builds a no-param sequenceState action command
// (pause/resume/finish).
func (s *Service) newEnrollmentActionCmd(token, action, short string) *cobra.Command {
	return &cobra.Command{
		Use:         action + " <id>",
		Short:       short,
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.runAction(cmd.Context(), token, enrollmentResource, args[0], action, nil)
		},
	}
}
