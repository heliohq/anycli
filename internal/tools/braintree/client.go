package braintree

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// Env var names the credential binding injects (definitions/tools/braintree.json).
const (
	EnvMerchantID  = "BRAINTREE_MERCHANT_ID"
	EnvPublicKey   = "BRAINTREE_PUBLIC_KEY"
	EnvPrivateKey  = "BRAINTREE_PRIVATE_KEY"
	EnvEnvironment = "BRAINTREE_ENVIRONMENT"
)

// braintreeVersion pins the Braintree-Version date header. Braintree returns
// the schema AS OF this date, so an old pin hides later types/fields. The
// official guidance is "use the date on which you begin integrating"; this pin
// is a current date chosen so every in-scope operation (ping, transaction
// search/get/refund/void/reverse, customer, dispute, subscription) resolves
// under one schema — the Dispute type postdates 2019-10-03, so a pre-dispute
// pin would make `dispute` verbs dead on arrival.
//
// Bump policy: raise this to a newer YYYY-MM-DD only after L2 re-confirms every
// in-scope verb still resolves under the new date against the live sandbox.
const braintreeVersion = "2025-06-01"

// Braintree GraphQL endpoints, one per environment. The same key pair is valid
// against exactly one environment, which is why environment is a credential
// field, not a flag.
const (
	sandboxHost    = "https://payments.sandbox.braintree-api.com/graphql"
	productionHost = "https://payments.braintree-api.com/graphql"
)

// usageError is a parameter / usage error (bad flag, missing required arg, a
// mutation supplied to the read-only `query` passthrough). It maps to exit code
// 2 and never issues a network request.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Braintree GraphQL error (HTTP 200 with a
// non-empty errors[] array), a non-2xx transport status, or a transport
// failure. It maps to exit code 1. status is the HTTP status (0 for transport
// failures). It wraps the cause so errors.As for a credential rejection still
// resolves through it.
type apiError struct {
	msg    string
	class  string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// client holds the resolved credentials and target host for one invocation.
// Tests set baseURL to an httptest server; production derives it from the
// injected environment field.
type client struct {
	baseURL    string
	authHeader string
	hc         *http.Client
}

// resolveBaseURL maps the injected BRAINTREE_ENVIRONMENT to the GraphQL host.
// Any value other than "sandbox"/"production" is rejected locally (no network
// call) — the same key pair is only valid against one environment, so a
// malformed environment is a credential-configuration error, not a usage error.
func resolveBaseURL(environment string) (string, error) {
	switch environment {
	case "sandbox":
		return sandboxHost, nil
	case "production":
		return productionHost, nil
	default:
		return "", &apiError{msg: fmt.Sprintf(
			"BRAINTREE_ENVIRONMENT must be \"sandbox\" or \"production\" (got %q)", environment)}
	}
}

// basicAuthHeader builds the HTTP Basic Authorization value from the API key
// pair: base64(public_key:private_key). The private key never appears in
// output — only this opaque header carries it.
func basicAuthHeader(publicKey, privateKey string) string {
	raw := publicKey + ":" + privateKey
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(raw))
}

// graphQLRequest is the standard GraphQL envelope.
type graphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

// graphQLResponse is the standard GraphQL response. Braintree returns HTTP 200
// with a non-empty errors[] on failure, so success is body-shaped, not
// status-shaped.
type graphQLResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []graphQLError  `json:"errors"`
}

type graphQLError struct {
	Message    string `json:"message"`
	Extensions struct {
		ErrorClass string `json:"errorClass"`
	} `json:"extensions"`
}

// do POSTs one {query, variables} body and returns the decoded data object. A
// non-empty errors[] (even under HTTP 200) becomes a typed apiError carrying
// errors[0].message and extensions.errorClass; a 401/403 additionally marks the
// credential rejected so the engine can invalidate it.
func (c *client) do(ctx context.Context, query string, variables map[string]any) (json.RawMessage, error) {
	payload, err := json.Marshal(graphQLRequest{Query: query, Variables: variables})
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("braintree: encode request: %v", err), err: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(payload))
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("braintree: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", c.authHeader)
	req.Header.Set("Braintree-Version", braintreeVersion)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("braintree: request: %v", err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("braintree: read response: %v", err), err: err}
	}

	// A hard non-2xx with no GraphQL body (auth/transport) is a credential or
	// transport failure. 401/403 are credential rejections.
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		raw := &apiError{msg: fmt.Sprintf("braintree: the provider rejected the credential (HTTP %d)", resp.StatusCode), status: resp.StatusCode}
		return nil, execution.RejectCredential(raw)
	}

	var decoded graphQLResponse
	if jsonErr := json.Unmarshal(body, &decoded); jsonErr != nil {
		return nil, &apiError{
			msg:    fmt.Sprintf("braintree: HTTP %d: %s", resp.StatusCode, truncate(string(body))),
			status: resp.StatusCode,
			err:    jsonErr,
		}
	}
	if len(decoded.Errors) > 0 {
		first := decoded.Errors[0]
		msg := fmt.Sprintf("braintree API error: %s", first.Message)
		if first.Extensions.ErrorClass != "" {
			msg = fmt.Sprintf("%s (%s)", msg, first.Extensions.ErrorClass)
		}
		return nil, &apiError{msg: msg, class: first.Extensions.ErrorClass, status: resp.StatusCode}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &apiError{
			msg:    fmt.Sprintf("braintree: HTTP %d: %s", resp.StatusCode, truncate(string(body))),
			status: resp.StatusCode,
		}
	}
	return decoded.Data, nil
}

// truncate bounds an error body so a large HTML/error page cannot flood output.
func truncate(s string) string {
	const max = 512
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}
