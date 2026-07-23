package zohocrm

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// parseRecordData parses the --data flag, which accepts either a single JSON
// object (one record) or a JSON array (bulk, up to 100 records), and returns
// the records as a slice ready to wrap into {"data":[…]}. Empty is a fail-fast
// usage error; invalid JSON is a usage error; a JSON value that is neither an
// object nor an array of objects is rejected.
func parseRecordData(val string) ([]json.RawMessage, error) {
	trimmed := strings.TrimSpace(val)
	if trimmed == "" {
		return nil, &usageError{msg: "--data is required (a JSON object, or a JSON array of objects)"}
	}
	var probe any
	if err := json.Unmarshal([]byte(trimmed), &probe); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("--data is not valid JSON: %v", err)}
	}
	switch probe.(type) {
	case map[string]any:
		return []json.RawMessage{json.RawMessage(trimmed)}, nil
	case []any:
		var arr []json.RawMessage
		if err := json.Unmarshal([]byte(trimmed), &arr); err != nil {
			return nil, &usageError{msg: fmt.Sprintf("--data is not valid JSON: %v", err)}
		}
		for _, el := range arr {
			var obj map[string]any
			if json.Unmarshal(el, &obj) != nil {
				return nil, &usageError{msg: "--data array must contain only JSON objects"}
			}
		}
		return arr, nil
	default:
		return nil, &usageError{msg: "--data must be a JSON object or a JSON array of objects"}
	}
}

// parseSingleObject parses a --data flag that must be a single JSON object
// (record update targets one id). It returns the object wrapped in a
// one-element slice for the {"data":[…]} envelope.
func parseSingleObject(val string) ([]json.RawMessage, error) {
	trimmed := strings.TrimSpace(val)
	if trimmed == "" {
		return nil, &usageError{msg: "--data is required (a single JSON object)"}
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("--data must be a single JSON object: %v", err)}
	}
	return []json.RawMessage{json.RawMessage(trimmed)}, nil
}

// setFieldsParam sets the `fields` query param (comma-joined API names, passed
// through verbatim) when non-empty.
func setFieldsParam(q url.Values, fields string) {
	if strings.TrimSpace(fields) != "" {
		q.Set("fields", fields)
	}
}

// requireModule validates the --module flag (the module API name, passed
// through verbatim so custom modules work).
func requireModule(module string) error {
	if strings.TrimSpace(module) == "" {
		return &usageError{msg: "--module is required (e.g. Leads, Contacts, Accounts, Deals, or a custom module API name)"}
	}
	return nil
}
