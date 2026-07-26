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
		Use:   "list",
		Short: "Search activities (POST /activities/search)",
		Long: "Activities are the logged interactions — notes, calls, meetings, emails.\n" +
			"Filtering is a search body like the record lists, but the typed `--name` and\n" +
			"`--email` flags do not apply to an activity: the filters that matter (the\n" +
			"parent record, a date range, an activity type) belong in `--json-body`.\n" +
			"Results are one page; step them with `--page` and `--page-size`.",
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
		Use:   "get",
		Short: "Get one activity by id (GET /activities/{id})",
		Long: "`--id` is required. Returns the logged interaction with its `details` text,\n" +
			"its type as a `{category, id}` pair, and the `parent` record it hangs off —\n" +
			"again a type plus an id, so the contact or deal itself needs its own `get`.\n" +
			"Translate the type id through `lookup activity-types`.",
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
		Use:   "create",
		Short: "Log an activity (POST /activities)",
		Long: "`--json-body` is required and needs three things: a `parent` naming the record\n" +
			"the activity belongs to (`{\"type\":\"person\",\"id\":123}`), a `type` given as the\n" +
			"`{\"category\":...,\"id\":...}` pair from `lookup activity-types`, and the\n" +
			"`details` text. The type pair cannot be invented — only `user` categories are\n" +
			"loggable, since `system` activities are Copper's own. Activities are immutable\n" +
			"once written, so there is no update verb and a mistake means delete and\n" +
			"re-log.",
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
		Use:   "delete",
		Short: "Delete an activity (DELETE /activities/{id})",
		Long: "`--id` is required. Because activities cannot be edited, this is the only way\n" +
			"to correct a wrongly logged interaction — delete it and log a replacement. The\n" +
			"deletion is permanent and removes the entry from the parent record's timeline,\n" +
			"so the interaction leaves no trace of having been recorded.",
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
