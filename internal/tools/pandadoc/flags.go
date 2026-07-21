package pandadoc

import (
	"fmt"
	"strings"
)

// recipient is one PandaDoc document recipient (create payload element).
type recipient struct {
	Email     string `json:"email,omitempty"`
	Role      string `json:"role,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
}

// token is one PandaDoc template variable (create payload element).
type token struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// parseRecipient parses the --recipient sugar `email[:role[:first[:last]]]`
// into a recipient. Only email is required.
func parseRecipient(raw string) (recipient, error) {
	parts := strings.Split(raw, ":")
	email := strings.TrimSpace(parts[0])
	if email == "" {
		return recipient{}, &usageError{msg: fmt.Sprintf("--recipient must start with an email, got %q", raw)}
	}
	r := recipient{Email: email}
	if len(parts) > 1 {
		r.Role = strings.TrimSpace(parts[1])
	}
	if len(parts) > 2 {
		r.FirstName = strings.TrimSpace(parts[2])
	}
	if len(parts) > 3 {
		r.LastName = strings.TrimSpace(parts[3])
	}
	return r, nil
}

// parseKeyValue splits a `name=value` pair. The value may contain further `=`.
func parseKeyValue(flag, raw string) (string, string, error) {
	name, value, ok := strings.Cut(raw, "=")
	if !ok || strings.TrimSpace(name) == "" {
		return "", "", &usageError{msg: fmt.Sprintf("--%s must be name=value, got %q", flag, raw)}
	}
	return strings.TrimSpace(name), value, nil
}

// buildTokens turns repeated --token name=value flags into the tokens array.
func buildTokens(raw []string) ([]token, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]token, 0, len(raw))
	for _, kv := range raw {
		name, value, err := parseKeyValue("token", kv)
		if err != nil {
			return nil, err
		}
		out = append(out, token{Name: name, Value: value})
	}
	return out, nil
}

// buildFields turns repeated --field name=value flags into the fields object,
// where each value is wrapped as {"value": <v>} per the PandaDoc create API.
func buildFields(raw []string) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(raw))
	for _, kv := range raw {
		name, value, err := parseKeyValue("field", kv)
		if err != nil {
			return nil, err
		}
		out[name] = map[string]any{"value": value}
	}
	return out, nil
}

// buildMetadata turns repeated --metadata k=v flags into the metadata object.
func buildMetadata(raw []string) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(raw))
	for _, kv := range raw {
		name, value, err := parseKeyValue("metadata", kv)
		if err != nil {
			return nil, err
		}
		out[name] = value
	}
	return out, nil
}
