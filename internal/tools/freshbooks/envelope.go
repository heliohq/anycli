package freshbooks

import (
	"encoding/json"
	"fmt"
)

// listEnvelope is the provider-neutral list shape the tool emits, unwrapping
// FreshBooks' {"response":{"result":{"<plural>":[...],"page":..,...}}} nesting.
type listEnvelope struct {
	Items   json.RawMessage `json:"items"`
	Page    int             `json:"page"`
	Pages   int             `json:"pages"`
	PerPage int             `json:"per_page"`
	Total   int             `json:"total"`
}

// unwrapList extracts the list under result.<plural> plus pagination fields from
// a FreshBooks accounting list body and returns the provider-neutral envelope.
func unwrapList(body []byte, plural string) (*listEnvelope, error) {
	var wrapper struct {
		Response struct {
			Result map[string]json.RawMessage `json:"result"`
		} `json:"response"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, &apiError{msg: fmt.Sprintf("freshbooks: decode list response: %v", err), err: err}
	}
	result := wrapper.Response.Result
	items := result[plural]
	if items == nil {
		items = json.RawMessage("[]")
	}
	env := &listEnvelope{Items: items}
	env.Page = intField(result["page"])
	env.Pages = intField(result["pages"])
	env.PerPage = intField(result["per_page"])
	env.Total = intField(result["total"])
	return env, nil
}

// unwrapObject extracts the single resource under result.<singular> from a
// FreshBooks accounting get/create/update body.
func unwrapObject(body []byte, singular string) (json.RawMessage, error) {
	var wrapper struct {
		Response struct {
			Result map[string]json.RawMessage `json:"result"`
		} `json:"response"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, &apiError{msg: fmt.Sprintf("freshbooks: decode response: %v", err), err: err}
	}
	obj := wrapper.Response.Result[singular]
	if obj == nil {
		return nil, &apiError{msg: fmt.Sprintf("freshbooks: response carried no %q object", singular)}
	}
	return obj, nil
}

// intField decodes a JSON number field to int, tolerating string-encoded
// numbers; absent or unparsable fields yield 0.
func intField(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		if i, err := n.Int64(); err == nil {
			return int(i)
		}
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		var sn json.Number = json.Number(s)
		if i, err := sn.Int64(); err == nil {
			return int(i)
		}
	}
	return 0
}
