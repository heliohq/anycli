package shopify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// usageError is a parameter / usage error (bad flag, invalid JSON, missing
// required flag). It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Shopify non-2xx, a GraphQL top-level
// error, a non-empty userErrors payload, or a transport failure. It maps to
// exit code 1 and kind "api". status is the HTTP status (0 for transport or
// GraphQL-layer failures). It wraps the cause so errors.As for a credential
// rejection still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// client carries the resolved credentials and store host for one invocation.
type client struct {
	svc   *Service
	token string
	store string
}

// graphResponse is the standard GraphQL over HTTP envelope. data is the
// operation result; errors are query-level (syntax/validation/throttling)
// failures returned even with HTTP 200.
type graphResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []graphError    `json:"errors"`
}

type graphError struct {
	Message string `json:"message"`
}

// endpoint builds the per-store GraphQL Admin endpoint, honoring a test
// BaseURL override, the --api-version flag, and the SHOPIFY_STORE host.
func (c *client) endpoint(apiVersion string) (string, error) {
	if c.svc.BaseURL != "" {
		return c.svc.BaseURL, nil
	}
	host := normalizeStore(c.store)
	if host == "" {
		return "", &usageError{msg: EnvStore + " is not set"}
	}
	v := apiVersion
	if v == "" {
		v = c.svc.APIVersion
	}
	if v == "" {
		v = DefaultAPIVersion
	}
	return "https://" + host + "/admin/api/" + v + "/graphql.json", nil
}

// gql POSTs a single GraphQL operation and returns the unwrapped `data` object.
// A non-2xx status, a GraphQL top-level error, or a decode failure surfaces as
// an apiError; a 401/403 is classified as a credential rejection so the token
// gateway's refresh path (design 227 A3) engages.
func (c *client) gql(ctx context.Context, apiVersion, query string, variables map[string]any) (map[string]any, error) {
	endpoint, err := c.endpoint(apiVersion)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{"query": query}
	if len(variables) > 0 {
		payload["variables"] = variables
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("shopify: encode request: %v", err), err: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("shopify: build request: %v", err), err: err}
	}
	req.Header.Set(accessTokenHeader, c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	hc := c.svc.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("shopify: POST %s: %v", endpoint, err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<22))
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("shopify: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		base := fmt.Errorf("shopify API error (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(raw)))
		classified := classifyCredentialError(resp.StatusCode, base)
		return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
	}
	var env graphResponse
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, &apiError{msg: fmt.Sprintf("shopify: decode response: %v", err), err: err}
	}
	if len(env.Errors) > 0 {
		return nil, &apiError{msg: fmt.Sprintf("shopify GraphQL error: %s", joinGraphErrors(env.Errors))}
	}
	var data map[string]any
	if len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, &data); err != nil {
			return nil, &apiError{msg: fmt.Sprintf("shopify: decode data: %v", err), err: err}
		}
	}
	return data, nil
}

// classifyCredentialError marks 401/403 as an explicit credential rejection so
// the host token gateway invalidates and refreshes the stored token.
func classifyCredentialError(status int, err error) error {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return execution.RejectCredential(err)
	}
	return err
}

// joinGraphErrors flattens GraphQL top-level error messages.
func joinGraphErrors(errs []graphError) string {
	msgs := make([]string, 0, len(errs))
	for _, e := range errs {
		if strings.TrimSpace(e.Message) != "" {
			msgs = append(msgs, e.Message)
		}
	}
	return strings.Join(msgs, "; ")
}

// userErrorsIn extracts a mutation payload's userErrors messages. Shopify
// returns HTTP 200 with a non-empty userErrors array on validation failure;
// the caller MUST treat a non-empty result as an exit-1 failure, never a
// silent no-op success. Handles the standard {field, message} shape.
func userErrorsIn(payload map[string]any) []string {
	rawList, ok := payload["userErrors"].([]any)
	if !ok || len(rawList) == 0 {
		return nil
	}
	msgs := make([]string, 0, len(rawList))
	for _, item := range rawList {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		msg, _ := m["message"].(string)
		field := fieldPath(m["field"])
		switch {
		case field != "" && msg != "":
			msgs = append(msgs, field+": "+msg)
		case msg != "":
			msgs = append(msgs, msg)
		}
	}
	return msgs
}

// fieldPath joins a userErrors `field` array (["variants", "0", "price"]) into
// a dotted path for a readable error.
func fieldPath(v any) string {
	list, ok := v.([]any)
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(list))
	for _, p := range list {
		if s, ok := p.(string); ok {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ".")
}

// mutationResult runs a mutation and returns the named payload node, failing
// (exit 1) when the payload's userErrors is non-empty.
func (c *client) mutationResult(ctx context.Context, apiVersion, mutation, payloadKey string, variables map[string]any) (map[string]any, error) {
	data, err := c.gql(ctx, apiVersion, mutation, variables)
	if err != nil {
		return nil, err
	}
	payload, _ := data[payloadKey].(map[string]any)
	if payload == nil {
		return nil, &apiError{msg: fmt.Sprintf("shopify: mutation %s returned no payload", payloadKey)}
	}
	if msgs := userErrorsIn(payload); len(msgs) > 0 {
		return nil, &apiError{msg: fmt.Sprintf("shopify mutation failed: %s", strings.Join(msgs, "; "))}
	}
	return payload, nil
}

// emit writes a value as compact JSON to stdout with a trailing newline.
func (c *client) emit(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("shopify: encode output: %v", err), err: err}
	}
	_, err = c.svc.stdout().Write(append(b, '\n'))
	return err
}

// connectionOut flattens a GraphQL connection ({edges:[{node}], pageInfo}) into
// the agent-friendly {<key>:[...nodes], page_info:{has_next_page,end_cursor}}
// envelope. A missing connection yields an empty list.
func connectionOut(data map[string]any, connKey, outKey string) map[string]any {
	nodes := []any{}
	pageInfo := map[string]any{"has_next_page": false, "end_cursor": nil}
	conn, _ := data[connKey].(map[string]any)
	if conn != nil {
		if edges, ok := conn["edges"].([]any); ok {
			for _, e := range edges {
				em, ok := e.(map[string]any)
				if !ok {
					continue
				}
				if node, ok := em["node"]; ok {
					nodes = append(nodes, node)
				}
			}
		}
		if pi, ok := conn["pageInfo"].(map[string]any); ok {
			pageInfo["has_next_page"] = pi["hasNextPage"]
			pageInfo["end_cursor"] = pi["endCursor"]
		}
	}
	return map[string]any{outKey: nodes, "page_info": pageInfo}
}

// gidOrRaw normalizes a bare numeric id into a Shopify GID for a given resource,
// or passes a full gid://… value through unchanged.
func gidOrRaw(resource, id string) string {
	id = strings.TrimSpace(id)
	if strings.HasPrefix(id, "gid://") {
		return id
	}
	return "gid://shopify/" + resource + "/" + id
}
