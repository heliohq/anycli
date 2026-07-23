package klaviyo

import (
	"encoding/json"
	"fmt"
)

// parseDataFlag validates a raw JSON:API request body from --data. An empty
// value yields nil (unset); invalid JSON is a fail-fast usage error. The value
// is passed through to the provider verbatim.
func parseDataFlag(val string) (json.RawMessage, error) {
	if val == "" {
		return nil, nil
	}
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(val), &raw); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("--data is not valid JSON: %v", err)}
	}
	return raw, nil
}

// resourceBody builds a JSON:API single-resource envelope
// {"data":{"type":<type>,"id":<id?>,"attributes":<attrs>,"relationships":<rels?>}}.
// id and relationships are omitted when empty.
func resourceBody(resourceType, id string, attributes map[string]any, relationships map[string]any) map[string]any {
	data := map[string]any{"type": resourceType}
	if id != "" {
		data["id"] = id
	}
	if len(attributes) > 0 {
		data["attributes"] = attributes
	}
	if len(relationships) > 0 {
		data["relationships"] = relationships
	}
	return map[string]any{"data": data}
}

// relationshipBody builds a JSON:API to-many relationship body
// {"data":[{"type":<type>,"id":<id>}, ...]} used by the list-membership
// relationship endpoints.
func relationshipBody(resourceType string, ids []string) map[string]any {
	data := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		data = append(data, map[string]any{"type": resourceType, "id": id})
	}
	return map[string]any{"data": data}
}

// singleRelationship builds a to-one relationship entry {"data":{"type","id"}}.
func singleRelationship(resourceType, id string) map[string]any {
	return map[string]any{"data": map[string]any{"type": resourceType, "id": id}}
}

// compactAttrs drops empty-string values so optional flags don't send blank
// attributes.
func compactAttrs(in map[string]string) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		if v != "" {
			out[k] = v
		}
	}
	return out
}
