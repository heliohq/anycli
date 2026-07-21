package reddit

import (
	"encoding/json"
	"fmt"
	"strings"
)

// jsonEnvelope is the api_type=json response shape used by Reddit's write
// endpoints (/api/submit, /api/comment, /api/editusertext, /api/compose).
// A non-empty errors array on an HTTP-200 response is a real failure.
type jsonEnvelope struct {
	JSON struct {
		Errors [][]any `json:"errors"`
		Data   struct {
			ID     string  `json:"id"`
			Name   string  `json:"name"`
			URL    string  `json:"url"`
			Things []thing `json:"things"`
		} `json:"data"`
	} `json:"json"`
}

// checkJSONErrors parses the api_type=json envelope and returns an apiError when
// the errors array is non-empty (Reddit reports these on HTTP 200). status is 0
// because there is no failing HTTP status — the dialect hides it in the body.
func checkJSONErrors(body []byte) (jsonEnvelope, error) {
	var env jsonEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return env, &apiError{msg: fmt.Sprintf("reddit: decode action response: %v", err), err: err}
	}
	if len(env.JSON.Errors) == 0 {
		return env, nil
	}
	parts := make([]string, 0, len(env.JSON.Errors))
	for _, e := range env.JSON.Errors {
		parts = append(parts, formatActionError(e))
	}
	return env, &apiError{msg: "reddit action failed: " + strings.Join(parts, "; ")}
}

// formatActionError renders one ["CODE","message","field"] tuple.
func formatActionError(e []any) string {
	str := func(i int) string {
		if i < len(e) {
			if v, ok := e[i].(string); ok {
				return v
			}
		}
		return ""
	}
	code, msg, field := str(0), str(1), str(2)
	switch {
	case field != "":
		return fmt.Sprintf("%s: %s (%s)", code, msg, field)
	case msg != "":
		return fmt.Sprintf("%s: %s", code, msg)
	default:
		return code
	}
}

// createdThing extracts the single thing created by /api/comment or
// /api/editusertext (returned under json.data.things[0]).
func createdThing(env jsonEnvelope) (thingData, bool) {
	for _, t := range env.JSON.Data.Things {
		d, err := decodeThingData(t.Data)
		if err == nil && d.Name != "" {
			return d, true
		}
	}
	return thingData{}, false
}
