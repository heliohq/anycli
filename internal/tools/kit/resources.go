package kit

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// sequenceCmd groups the sequence (automation) commands: list sequences and
// enroll a subscriber into one.
func (s *Service) sequenceCmd(token string) *cobra.Command {
	group := newGroupCmd("sequence", "Manage sequences (automations)")

	list := &cobra.Command{
		Use:         "list",
		Short:       "List sequences (one page; use --after to continue)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	lf := registerListFlags(list)
	list.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		lf.apply(q)
		body, err := s.call(cmd.Context(), token, http.MethodGet, "/sequences", q, nil)
		if err != nil {
			return err
		}
		return s.emitData(body, "sequences")
	}

	var sequenceID, subscriberID int
	var email string
	add := &cobra.Command{
		Use:         "add",
		Short:       "Enroll a subscriber into a sequence",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requirePositive("sequence-id", sequenceID); err != nil {
				return err
			}
			suffix, q, reqBody, err := membershipRequest(http.MethodPost, subscriberID, email)
			if err != nil {
				return err
			}
			path := "/sequences/" + strconv.Itoa(sequenceID) + "/subscribers" + suffix
			body, callErr := s.call(cmd.Context(), token, http.MethodPost, path, q, bodyOrNil(reqBody))
			if callErr != nil {
				return callErr
			}
			return s.emitData(body, "subscriber")
		},
	}
	add.Flags().IntVar(&sequenceID, "sequence-id", 0, "sequence id (required)")
	add.Flags().IntVar(&subscriberID, "subscriber-id", 0, "subscriber id (XOR --email)")
	add.Flags().StringVar(&email, "email", "", "subscriber email (XOR --subscriber-id)")

	group.AddCommand(list, add)
	return group
}

// formCmd groups the form commands: list forms and subscribe a contact via a
// form (Kit's canonical opt-in path).
func (s *Service) formCmd(token string) *cobra.Command {
	group := newGroupCmd("form", "Manage forms and form subscriptions")

	list := &cobra.Command{
		Use:         "list",
		Short:       "List forms (one page; use --after to continue)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	lf := registerListFlags(list)
	list.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		lf.apply(q)
		body, err := s.call(cmd.Context(), token, http.MethodGet, "/forms", q, nil)
		if err != nil {
			return err
		}
		return s.emitData(body, "forms")
	}

	var formID, subscriberID int
	var email string
	add := &cobra.Command{
		Use:         "add",
		Short:       "Subscribe a contact to a form",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requirePositive("form-id", formID); err != nil {
				return err
			}
			suffix, q, reqBody, err := membershipRequest(http.MethodPost, subscriberID, email)
			if err != nil {
				return err
			}
			path := "/forms/" + strconv.Itoa(formID) + "/subscribers" + suffix
			body, callErr := s.call(cmd.Context(), token, http.MethodPost, path, q, bodyOrNil(reqBody))
			if callErr != nil {
				return callErr
			}
			return s.emitData(body, "subscriber")
		},
	}
	add.Flags().IntVar(&formID, "form-id", 0, "form id (required)")
	add.Flags().IntVar(&subscriberID, "subscriber-id", 0, "subscriber id (XOR --email)")
	add.Flags().StringVar(&email, "email", "", "subscriber email (XOR --subscriber-id)")

	group.AddCommand(list, add)
	return group
}

// customFieldCmd groups the custom-field commands: read/create the arbitrary
// per-subscriber data model.
func (s *Service) customFieldCmd(token string) *cobra.Command {
	group := newGroupCmd("custom-field", "Manage custom fields")

	list := &cobra.Command{
		Use:         "list",
		Short:       "List custom fields (one page; use --after to continue)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	lf := registerListFlags(list)
	list.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		lf.apply(q)
		body, err := s.call(cmd.Context(), token, http.MethodGet, "/custom_fields", q, nil)
		if err != nil {
			return err
		}
		return s.emitData(body, "custom_fields")
	}

	var label string
	create := &cobra.Command{
		Use:         "create",
		Short:       "Create a custom field",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if label == "" {
				return &usageError{msg: "--label is required"}
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/custom_fields", nil, map[string]any{"label": label})
			if err != nil {
				return err
			}
			return s.emitData(body, "custom_field")
		},
	}
	create.Flags().StringVar(&label, "label", "", "custom field label (required)")

	group.AddCommand(list, create)
	return group
}

// segmentCmd groups the segment commands: enumerate saved segments for
// broadcast targeting.
func (s *Service) segmentCmd(token string) *cobra.Command {
	group := newGroupCmd("segment", "Enumerate saved segments")

	list := &cobra.Command{
		Use:         "list",
		Short:       "List segments (one page; use --after to continue)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	lf := registerListFlags(list)
	list.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		lf.apply(q)
		body, err := s.call(cmd.Context(), token, http.MethodGet, "/segments", q, nil)
		if err != nil {
			return err
		}
		return s.emitData(body, "segments")
	}

	group.AddCommand(list)
	return group
}

// bodyOrNil returns the map as an any payload, or nil when empty, so a
// membership POST by subscriber id sends no body.
func bodyOrNil(m map[string]any) any {
	if len(m) == 0 {
		return nil
	}
	return m
}
