package klaviyo

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/spf13/cobra"
)

// The event read Longs, beside this group because the generic builders in
// common.go cannot say what an event is or which filter makes the list usable.
const (
	longEventList = "Events are the individual metric occurrences behind every report, and the\n" +
		"filter is what makes this usable: `equals(profile_id,\"<id>\")`,\n" +
		"`equals(metric_id,\"<id>\")`, `greater-than(datetime,2026-01-01T00:00:00Z)`.\n" +
		"Without one this pages through the account's entire event firehose."

	longEventGet = "One event with its properties, and its metric and profile as relationship\n" +
		"ids rather than objects — pass `--include metric,profile` to get them in\n" +
		"the same response instead of making two more calls."
)

// newEventCmd builds the `event` group: list/get plus create (which triggers
// flows).
func (s *Service) newEventCmd(token string) *cobra.Command {
	group := newGroupCmd("event", "Read events and create custom events")
	group.AddCommand(
		s.newCollectionListCmd(token, "list", "List events (GET /events)", longEventList, "/events", "event"),
		s.newResourceGetCmd(token, "get", "Get one event (GET /events/{id})", longEventGet, "/events/", "event"),
		s.newEventCreateCmd(token),
	)
	return group
}

// newEventCreateCmd builds `event create` → POST /events. The convenience path
// takes --metric (name), --email (profile), optional --value and --properties;
// --data overrides with a raw JSON:API body.
func (s *Service) newEventCreateCmd(token string) *cobra.Command {
	var metric, email, value, properties, data string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a custom event (POST /events) via --metric/--email or --data",
		Long: "--metric (the metric NAME, created on first use if the account has never\n" +
			"seen it) and --email are both required unless --data carries a full body.\n" +
			"--value is numeric and --properties a JSON object. Recording an event can\n" +
			"TRIGGER any live flow waiting on that metric, so this reaches real\n" +
			"customers even though nothing about it looks like a send. The endpoint\n" +
			"answers with an empty body, so a local receipt is printed instead.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := eventCreateBody(metric, email, value, properties, data)
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/events", nil, payload)
			if err != nil {
				return err
			}
			if len(body) == 0 {
				return s.emit([]byte(`{"status":"ok"}`))
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&metric, "metric", "", "metric name for the event")
	cmd.Flags().StringVar(&email, "email", "", "profile email the event belongs to")
	cmd.Flags().StringVar(&value, "value", "", "numeric event value (optional, e.g. order total)")
	cmd.Flags().StringVar(&properties, "properties", "", "event properties as a JSON object (optional)")
	cmd.Flags().StringVar(&data, "data", "", "raw JSON:API request body (overrides the shorthand)")
	return cmd
}

// eventCreateBody builds the event payload. --data wins verbatim; otherwise it
// requires --metric and --email and assembles the metric/profile relationships
// inline (Klaviyo's create-event shape nests them under attributes).
func eventCreateBody(metric, email, value, properties, data string) (any, error) {
	if data != "" {
		return parseDataFlag(data)
	}
	if metric == "" || email == "" {
		return nil, &usageError{msg: "provide both --metric and --email, or --data"}
	}
	attrs := map[string]any{
		"metric":  map[string]any{"data": map[string]any{"type": "metric", "attributes": map[string]any{"name": metric}}},
		"profile": map[string]any{"data": map[string]any{"type": "profile", "attributes": map[string]any{"email": email}}},
	}
	if value != "" {
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, &usageError{msg: "--value must be a number, got " + value}
		}
		attrs["value"] = v
	}
	if properties != "" {
		var props map[string]any
		if err := json.Unmarshal([]byte(properties), &props); err != nil {
			return nil, &usageError{msg: "--properties is not a valid JSON object: " + err.Error()}
		}
		attrs["properties"] = props
	}
	return resourceBody("event", "", attrs, nil), nil
}
