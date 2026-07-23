package novu

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newEventCmd is the `event` group: the core send surface (POST /v1/events/*).
// A successful trigger returns {"data":{acknowledged,status,error,transactionId,
// activityFeedLink,…}} — HTTP 201 means Novu ACCEPTED the trigger, not that it
// was delivered. The load-bearing outcome is the `status` field
// (processed = success; trigger_not_active / no_workflow_*_steps_defined /
// no_tenant_found / invalid_recipients / error = not delivered). emit passes the
// whole envelope through so the agent sees status, never just transactionId.
func (s *Service) newEventCmd(c *client) *cobra.Command {
	group := newGroupCmd("event", "Trigger notifications (send)")
	group.AddCommand(
		s.newEventTriggerCmd(c),
		s.newEventBulkCmd(c),
		s.newEventBroadcastCmd(c),
		s.newEventCancelCmd(c),
	)
	return group
}

func (s *Service) newEventTriggerCmd(c *client) *cobra.Command {
	var workflow, to, toJSON, payload, overrides, transactionID, actor, tenant string
	cmd := leafCmd("trigger", "Trigger a workflow to a subscriber or topic", writeAction, func(cmd *cobra.Command, _ []string) error {
		if err := requireFlag("workflow", workflow); err != nil {
			return err
		}
		body := map[string]any{"name": workflow}

		recipient, err := resolveRecipient(to, toJSON)
		if err != nil {
			return err
		}
		if recipient == nil {
			return &usageError{msg: "novu: one of --to or --to-json is required"}
		}
		body["to"] = recipient

		if err := putJSON(body, "payload", payload); err != nil {
			return err
		}
		if err := putJSON(body, "overrides", overrides); err != nil {
			return err
		}
		setIfNonEmpty(body, "transactionId", transactionID)
		// actor / tenant accept either a bare id string or a JSON object.
		if err := putScalarOrJSON(body, "actor", actor); err != nil {
			return err
		}
		if err := putScalarOrJSON(body, "tenant", tenant); err != nil {
			return err
		}

		out, err := c.call(cmd.Context(), http.MethodPost, "/v1/events/trigger", nil, body)
		if err != nil {
			return err
		}
		return s.emit(out)
	})
	f := cmd.Flags()
	f.StringVar(&workflow, "workflow", "", "workflow trigger identifier (required)")
	f.StringVar(&to, "to", "", "recipient subscriberId (simple case)")
	f.StringVar(&toJSON, "to-json", "", "recipient as raw JSON (subscriber object, topic, or array; up to 100)")
	f.StringVar(&payload, "payload", "", "workflow payload as a JSON object")
	f.StringVar(&overrides, "overrides", "", "provider overrides as a JSON object")
	f.StringVar(&transactionID, "transaction-id", "", "idempotency key for deduplication")
	f.StringVar(&actor, "actor", "", "actor subscriberId or JSON subscriber object")
	f.StringVar(&tenant, "tenant", "", "tenant identifier or JSON tenant object")
	return cmd
}

func (s *Service) newEventBulkCmd(c *client) *cobra.Command {
	var events string
	cmd := leafCmd("bulk", "Trigger up to 100 events in one call", writeAction, func(cmd *cobra.Command, _ []string) error {
		decoded, err := decodeJSONFlag("events", events)
		if err != nil {
			return err
		}
		if decoded == nil {
			return &usageError{msg: "novu: --events is required (a JSON array of trigger objects)"}
		}
		out, err := c.call(cmd.Context(), http.MethodPost, "/v1/events/trigger/bulk", nil, map[string]any{"events": decoded})
		if err != nil {
			return err
		}
		return s.emit(out)
	})
	cmd.Flags().StringVar(&events, "events", "", "JSON array of trigger objects (required)")
	return cmd
}

func (s *Service) newEventBroadcastCmd(c *client) *cobra.Command {
	var workflow, payload, overrides string
	cmd := leafCmd("broadcast", "Trigger a workflow to every subscriber", writeAction, func(cmd *cobra.Command, _ []string) error {
		if err := requireFlag("workflow", workflow); err != nil {
			return err
		}
		body := map[string]any{"name": workflow}
		if err := putJSON(body, "payload", payload); err != nil {
			return err
		}
		if err := putJSON(body, "overrides", overrides); err != nil {
			return err
		}
		out, err := c.call(cmd.Context(), http.MethodPost, "/v1/events/trigger/broadcast", nil, body)
		if err != nil {
			return err
		}
		return s.emit(out)
	})
	f := cmd.Flags()
	f.StringVar(&workflow, "workflow", "", "workflow trigger identifier (required)")
	f.StringVar(&payload, "payload", "", "workflow payload as a JSON object")
	f.StringVar(&overrides, "overrides", "", "provider overrides as a JSON object")
	return cmd
}

func (s *Service) newEventCancelCmd(c *client) *cobra.Command {
	var transactionID string
	cmd := leafCmd("cancel", "Cancel a triggered event by transaction id", writeAction, func(cmd *cobra.Command, _ []string) error {
		if err := requireFlag("transaction-id", transactionID); err != nil {
			return err
		}
		out, err := c.call(cmd.Context(), http.MethodDelete, "/v1/events/trigger/"+pathEscape(transactionID), nil, nil)
		if err != nil {
			return err
		}
		return s.emit(out)
	})
	cmd.Flags().StringVar(&transactionID, "transaction-id", "", "transaction id to cancel (required)")
	return cmd
}

// resolveRecipient prefers the raw-JSON form (--to-json) for topics/arrays/rich
// subscriber objects, falling back to the bare subscriberId string (--to).
func resolveRecipient(to, toJSON string) (any, error) {
	if toJSON != "" {
		return decodeJSONFlag("to-json", toJSON)
	}
	if to != "" {
		return to, nil
	}
	return nil, nil
}

// putJSON decodes a JSON-object flag into body[key], omitting it when empty.
func putJSON(body map[string]any, key, raw string) error {
	v, err := decodeJSONFlag(key, raw)
	if err != nil {
		return err
	}
	if v != nil {
		body[key] = v
	}
	return nil
}

// putScalarOrJSON accepts either a bare string (used verbatim) or, when the
// value parses as JSON, the decoded object — matching Novu fields (actor,
// tenant) that accept "id" or {id,…}.
func putScalarOrJSON(body map[string]any, key, raw string) error {
	if raw == "" {
		return nil
	}
	var v any
	if err := jsonUnmarshalStrict(raw, &v); err == nil {
		body[key] = v
		return nil
	}
	body[key] = raw
	return nil
}
