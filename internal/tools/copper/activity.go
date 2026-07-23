package copper

import (
	"net/http"
	"strconv"

	"github.com/spf13/cobra"
)

// newActivityCmd exposes activity logging (notes, calls, emails). Activities
// support search / get / create / delete but not update — Copper activities are
// immutable once logged.
func (s *Service) newActivityCmd(token string) *cobra.Command {
	group := newGroupCmd("activity", "Activities (notes, calls, emails)")
	group.AddCommand(
		s.newActivityListCmd(token),
		s.newActivityGetCmd(token),
		s.newActivityCreateCmd(token),
		s.newActivityDeleteCmd(token),
	)
	return group
}

func (s *Service) newActivityListCmd(token string) *cobra.Command {
	var f searchFlags
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "Search activities (POST /activities/search)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := f.searchBody()
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/activities/search", body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerSearchFlags(cmd, &f)
	return cmd
}

func (s *Service) newActivityGetCmd(token string) *cobra.Command {
	var id int
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get one activity by id (GET /activities/{id})",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if id <= 0 {
				return &usageError{msg: "--id is required"}
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/activities/"+strconv.Itoa(id), nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&id, "id", 0, "Copper activity id")
	return cmd
}

func (s *Service) newActivityCreateCmd(token string) *cobra.Command {
	var jsonBody string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Log an activity (POST /activities)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if jsonBody == "" {
				return &usageError{msg: "--json-body is required (the activity payload: type, parent, details)"}
			}
			body, err := decodeJSONBody(jsonBody)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/activities", body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&jsonBody, "json-body", "", "raw JSON activity payload")
	return cmd
}

func (s *Service) newActivityDeleteCmd(token string) *cobra.Command {
	var id int
	cmd := &cobra.Command{
		Use:         "delete",
		Short:       "Delete an activity (DELETE /activities/{id})",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if id <= 0 {
				return &usageError{msg: "--id is required"}
			}
			resp, err := s.call(cmd.Context(), token, http.MethodDelete, "/activities/"+strconv.Itoa(id), nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&id, "id", 0, "Copper activity id")
	return cmd
}
