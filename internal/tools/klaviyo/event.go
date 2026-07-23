package klaviyo

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/spf13/cobra"
)

// newEventCmd builds the `event` group: list/get plus create (which triggers
// flows).
func (s *Service) newEventCmd(token string) *cobra.Command {
	group := newGroupCmd("event", "Read events and create custom events")
	group.AddCommand(
		s.newCollectionListCmd(token, "list", "List events (GET /events)", "/events", "event"),
		s.newResourceGetCmd(token, "get", "Get one event (GET /events/{id})", "/events/", "event"),
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
		Use:         "create",
		Short:       "Create a custom event (POST /events) via --metric/--email or --data",
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
