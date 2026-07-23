package knock

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newScheduleCmd groups the schedule verbs: send a workflow later or on a
// recurring cadence.
func (s *Service) newScheduleCmd(key string) *cobra.Command {
	group := newGroupCmd("schedule", "Schedule workflows to run later or recurringly")
	group.AddCommand(
		s.newScheduleCreateCmd(key),
		s.newScheduleListCmd(key),
		s.newScheduleUpdateCmd(key),
		s.newScheduleDeleteCmd(key),
	)
	return group
}

func (s *Service) newScheduleCreateCmd(key string) *cobra.Command {
	var (
		recipient   []string
		workflow    string
		scheduledAt string
		repeats     string
		data        string
		tenant      string
		actor       string
		endingAt    string
	)
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create schedules for one or more recipients",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID("workflow", workflow); err != nil {
				return err
			}
			if len(recipient) == 0 {
				return &usageError{msg: "at least one --recipient is required"}
			}
			if scheduledAt == "" && repeats == "" {
				return &usageError{msg: "provide --scheduled-at and/or --repeats"}
			}
			body := map[string]any{
				"recipients": recipient,
				"workflow":   workflow,
			}
			if scheduledAt != "" {
				body["scheduled_at"] = scheduledAt
			}
			if repeats != "" {
				parsed, decErr := decodeJSONFlag("repeats", repeats)
				if decErr != nil {
					return decErr
				}
				if _, ok := parsed.([]any); !ok {
					return &usageError{msg: "--repeats must be a JSON array of repeat rules"}
				}
				body["repeats"] = parsed
			}
			if data != "" {
				parsed, decErr := decodeJSONFlag("data", data)
				if decErr != nil {
					return decErr
				}
				body["data"] = parsed
			}
			if tenant != "" {
				body["tenant"] = tenant
			}
			if actor != "" {
				body["actor"] = actor
			}
			if endingAt != "" {
				body["ending_at"] = endingAt
			}
			return s.callEmit(cmd.Context(), key, http.MethodPost, "/schedules", nil, body, nil)
		},
	}
	cmd.Flags().StringArrayVar(&recipient, "recipient", nil, "recipient id (repeatable, required)")
	cmd.Flags().StringVar(&workflow, "workflow", "", "workflow key to schedule (required)")
	cmd.Flags().StringVar(&scheduledAt, "scheduled-at", "", "ISO-8601 one-time run time")
	cmd.Flags().StringVar(&repeats, "repeats", "", "recurrence rules as a JSON array")
	cmd.Flags().StringVar(&data, "data", "", "workflow payload as a JSON object")
	cmd.Flags().StringVar(&tenant, "tenant", "", "tenant id scoping the schedule")
	cmd.Flags().StringVar(&actor, "actor", "", "actor id")
	cmd.Flags().StringVar(&endingAt, "ending-at", "", "ISO-8601 time after which the schedule stops")
	return cmd
}

func (s *Service) newScheduleListCmd(key string) *cobra.Command {
	var (
		workflow string
		tenant   string
		pageSize int
		after    string
		before   string
	)
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List schedules",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID("workflow", workflow); err != nil {
				return err
			}
			q := url.Values{}
			q.Set("workflow", workflow)
			if tenant != "" {
				q.Set("tenant", tenant)
			}
			addPaging(q, pageSize, after, before)
			return s.callEmit(cmd.Context(), key, http.MethodGet, "/schedules", q, nil, nil)
		},
	}
	cmd.Flags().StringVar(&workflow, "workflow", "", "workflow key (required)")
	cmd.Flags().StringVar(&tenant, "tenant", "", "filter by tenant id")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "page size (Knock default 50)")
	cmd.Flags().StringVar(&after, "after", "", "pagination cursor (next page)")
	cmd.Flags().StringVar(&before, "before", "", "pagination cursor (previous page)")
	return cmd
}

func (s *Service) newScheduleUpdateCmd(key string) *cobra.Command {
	var (
		scheduleID  []string
		scheduledAt string
		repeats     string
		data        string
		tenant      string
		actor       string
		endingAt    string
	)
	cmd := &cobra.Command{
		Use:         "update",
		Short:       "Update existing schedules by id",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if len(scheduleID) == 0 {
				return &usageError{msg: "at least one --schedule-id is required"}
			}
			body := map[string]any{"schedule_ids": scheduleID}
			if scheduledAt != "" {
				body["scheduled_at"] = scheduledAt
			}
			if repeats != "" {
				parsed, decErr := decodeJSONFlag("repeats", repeats)
				if decErr != nil {
					return decErr
				}
				if _, ok := parsed.([]any); !ok {
					return &usageError{msg: "--repeats must be a JSON array of repeat rules"}
				}
				body["repeats"] = parsed
			}
			if data != "" {
				parsed, decErr := decodeJSONFlag("data", data)
				if decErr != nil {
					return decErr
				}
				body["data"] = parsed
			}
			if tenant != "" {
				body["tenant"] = tenant
			}
			if actor != "" {
				body["actor"] = actor
			}
			if endingAt != "" {
				body["ending_at"] = endingAt
			}
			return s.callEmit(cmd.Context(), key, http.MethodPut, "/schedules", nil, body, nil)
		},
	}
	cmd.Flags().StringArrayVar(&scheduleID, "schedule-id", nil, "schedule id to update (repeatable, required)")
	cmd.Flags().StringVar(&scheduledAt, "scheduled-at", "", "ISO-8601 one-time run time")
	cmd.Flags().StringVar(&repeats, "repeats", "", "recurrence rules as a JSON array")
	cmd.Flags().StringVar(&data, "data", "", "workflow payload as a JSON object")
	cmd.Flags().StringVar(&tenant, "tenant", "", "tenant id scoping the schedule")
	cmd.Flags().StringVar(&actor, "actor", "", "actor id")
	cmd.Flags().StringVar(&endingAt, "ending-at", "", "ISO-8601 time after which the schedule stops")
	return cmd
}

func (s *Service) newScheduleDeleteCmd(key string) *cobra.Command {
	var scheduleID []string
	cmd := &cobra.Command{
		Use:         "delete",
		Short:       "Delete schedules by id",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if len(scheduleID) == 0 {
				return &usageError{msg: "at least one --schedule-id is required"}
			}
			body := map[string]any{"schedule_ids": scheduleID}
			return s.callEmit(cmd.Context(), key, http.MethodDelete, "/schedules", nil, body, nil)
		},
	}
	cmd.Flags().StringArrayVar(&scheduleID, "schedule-id", nil, "schedule id to delete (repeatable, required)")
	return cmd
}
