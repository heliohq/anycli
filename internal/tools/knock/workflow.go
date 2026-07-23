package knock

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newWorkflowCmd groups the workflow trigger/cancel verbs — the #1 job: send a
// notification by running a configured workflow.
func (s *Service) newWorkflowCmd(key string) *cobra.Command {
	group := newGroupCmd("workflow", "Trigger and cancel notification workflows")
	group.AddCommand(s.newWorkflowTriggerCmd(key), s.newWorkflowCancelCmd(key))
	return group
}

func (s *Service) newWorkflowTriggerCmd(key string) *cobra.Command {
	var (
		workflowKey     string
		recipient       []string
		recipientsJSON  string
		data            string
		actor           string
		tenant          string
		cancellationKey string
		sandbox         bool
		skipDelay       bool
		idempotencyKey  string
	)
	cmd := &cobra.Command{
		Use:         "trigger",
		Short:       "Trigger a workflow to notify one or more recipients",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID("key", workflowKey); err != nil {
				return err
			}
			recipients, err := buildRecipients(recipient, recipientsJSON)
			if err != nil {
				return err
			}
			// Trigger safety (design decision 2): never fan out to an empty
			// audience — require an explicit recipient.
			if len(recipients) == 0 {
				return &usageError{msg: "at least one --recipient (or --recipients-json) is required"}
			}
			body := map[string]any{"recipients": recipients}
			if data != "" {
				parsed, decErr := decodeJSONFlag("data", data)
				if decErr != nil {
					return decErr
				}
				body["data"] = parsed
			}
			if actor != "" {
				body["actor"] = actor
			}
			if tenant != "" {
				body["tenant"] = tenant
			}
			if cancellationKey != "" {
				body["cancellation_key"] = cancellationKey
			}
			settings := map[string]any{}
			if sandbox {
				settings["sandbox_mode"] = true
			}
			if skipDelay {
				settings["skip_delay"] = true
			}
			if len(settings) > 0 {
				body["settings"] = settings
			}
			var headers map[string]string
			if idempotencyKey != "" {
				headers = map[string]string{"Idempotency-Key": idempotencyKey}
			}
			return s.callEmit(cmd.Context(), key, http.MethodPost, "/workflows/"+url.PathEscape(workflowKey)+"/trigger", nil, body, headers)
		},
	}
	cmd.Flags().StringVar(&workflowKey, "key", "", "workflow key to trigger (required)")
	cmd.Flags().StringArrayVar(&recipient, "recipient", nil, "recipient id (repeatable)")
	cmd.Flags().StringVar(&recipientsJSON, "recipients-json", "", "recipients as a raw JSON array (advanced: inline recipient objects)")
	cmd.Flags().StringVar(&data, "data", "", "workflow payload as a JSON object")
	cmd.Flags().StringVar(&actor, "actor", "", "id of the actor that triggered the workflow")
	cmd.Flags().StringVar(&tenant, "tenant", "", "tenant id scoping the trigger")
	cmd.Flags().StringVar(&cancellationKey, "cancellation-key", "", "key used to later cancel these queued runs")
	cmd.Flags().BoolVar(&sandbox, "sandbox", false, "sandbox_mode: simulate the run without delivering (safe dry-run)")
	cmd.Flags().BoolVar(&skipDelay, "skip-delay", false, "skip workflow delay steps")
	cmd.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "Idempotency-Key header (24h dedup window)")
	return cmd
}

func (s *Service) newWorkflowCancelCmd(key string) *cobra.Command {
	var (
		workflowKey     string
		cancellationKey string
		recipient       []string
	)
	cmd := &cobra.Command{
		Use:         "cancel",
		Short:       "Cancel queued workflow runs by cancellation key",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID("key", workflowKey); err != nil {
				return err
			}
			if err := requireID("cancellation-key", cancellationKey); err != nil {
				return err
			}
			body := map[string]any{"cancellation_key": cancellationKey}
			if len(recipient) > 0 {
				body["recipients"] = recipient
			}
			return s.callEmit(cmd.Context(), key, http.MethodPost, "/workflows/"+url.PathEscape(workflowKey)+"/cancel", nil, body, nil)
		},
	}
	cmd.Flags().StringVar(&workflowKey, "key", "", "workflow key (required)")
	cmd.Flags().StringVar(&cancellationKey, "cancellation-key", "", "cancellation key used at trigger time (required)")
	cmd.Flags().StringArrayVar(&recipient, "recipient", nil, "limit cancellation to these recipient ids (repeatable)")
	return cmd
}

// buildRecipients merges the simple repeatable --recipient ids with an optional
// raw JSON array of recipient objects. Supplying both is rejected to keep the
// audience unambiguous.
func buildRecipients(recipient []string, recipientsJSON string) ([]any, error) {
	if recipientsJSON != "" {
		if len(recipient) > 0 {
			return nil, &usageError{msg: "provide either --recipient or --recipients-json, not both"}
		}
		parsed, err := decodeJSONFlag("recipients-json", recipientsJSON)
		if err != nil {
			return nil, err
		}
		arr, ok := parsed.([]any)
		if !ok {
			return nil, &usageError{msg: "--recipients-json must be a JSON array"}
		}
		return arr, nil
	}
	recipients := make([]any, 0, len(recipient))
	for _, r := range recipient {
		recipients = append(recipients, r)
	}
	return recipients, nil
}
