package braintree

import (
	"encoding/json"
	"fmt"
)

// searchResult is the provider-neutral list shape: GraphQL edges[].node
// flattened into items, with the connection's pageInfo surfaced for cursoring.
type searchResult struct {
	Items    []any       `json:"items"`
	PageInfo pageInfoOut `json:"page_info"`
}

type pageInfoOut struct {
	HasNextPage bool   `json:"has_next_page"`
	EndCursor   string `json:"end_cursor"`
}

// decodeConnection flattens data.search.<field> (a GraphQL Relay connection)
// into a searchResult. field is the connection selection ("transactions",
// "customers", "disputes").
func decodeConnection(data json.RawMessage, field string) (searchResult, error) {
	var envelope struct {
		Search map[string]struct {
			PageInfo struct {
				HasNextPage bool   `json:"hasNextPage"`
				EndCursor   string `json:"endCursor"`
			} `json:"pageInfo"`
			Edges []struct {
				Node any `json:"node"`
			} `json:"edges"`
		} `json:"search"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return searchResult{}, &apiError{msg: fmt.Sprintf("braintree: decode %s search response: %v", field, err), err: err}
	}
	conn := envelope.Search[field]
	items := make([]any, 0, len(conn.Edges))
	for _, e := range conn.Edges {
		items = append(items, e.Node)
	}
	return searchResult{
		Items: items,
		PageInfo: pageInfoOut{
			HasNextPage: conn.PageInfo.HasNextPage,
			EndCursor:   conn.PageInfo.EndCursor,
		},
	}, nil
}

// decodeNode extracts a single object at data.<field>. A null value (e.g. a
// node(id) lookup whose id does not resolve to the expected type) is a runtime
// not-found, surfaced as an apiError (exit 1).
func decodeNode(data json.RawMessage, field, notFoundMsg string) (any, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, &apiError{msg: fmt.Sprintf("braintree: decode %s response: %v", field, err), err: err}
	}
	raw, ok := envelope[field]
	if !ok || string(raw) == "null" {
		return nil, &apiError{msg: notFoundMsg}
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, &apiError{msg: fmt.Sprintf("braintree: decode %s payload: %v", field, err), err: err}
	}
	return value, nil
}

// decodeNestedNode extracts data.<outer>.<inner> (mutation payload → its result
// object, e.g. refundTransaction.refund, voidTransaction.transaction).
func decodeNestedNode(data json.RawMessage, outer, inner string) (any, error) {
	var envelope map[string]map[string]any
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, &apiError{msg: fmt.Sprintf("braintree: decode %s response: %v", outer, err), err: err}
	}
	payload, ok := envelope[outer]
	if !ok || payload == nil {
		return nil, &apiError{msg: fmt.Sprintf("braintree: %s returned no payload", outer)}
	}
	value, ok := payload[inner]
	if !ok || value == nil {
		return nil, &apiError{msg: fmt.Sprintf("braintree: %s returned no %s", outer, inner)}
	}
	return value, nil
}
