// Package msgraph holds helpers shared by the Microsoft Graph-backed tools
// (microsoft-outlook, microsoft-calendar, microsoft-onedrive), which all hit
// the same Entra/Graph backend and must classify token errors identically.
package msgraph

import (
	"encoding/json"
	"net/http"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// errorEnvelope is Graph's error shape: {"error":{"code":"...","message":"..."}}.
type errorEnvelope struct {
	Error struct {
		Code string `json:"code"`
	} `json:"error"`
}

// tokenErrorCodes is the canonical set of Microsoft Graph / Entra error codes
// that indicate the access token itself was rejected (as opposed to a valid
// token that merely lacks a scope, which surfaces as HTTP 403). All three
// Graph-backed tools share this set so an identical error body is classified
// the same way everywhere.
var tokenErrorCodes = map[string]bool{
	"InvalidAuthenticationToken":   true,
	"CompactToken_ParsingFailed":   true,
	"CompactToken_InvalidAudience": true,
	"TokenExpired":                 true,
	"AuthenticationError":          true,
}

// ClassifyCredentialError marks a response as a credential rejection when the
// status is 401 or Graph reports one of tokenErrorCodes, so the token gateway
// can prompt a reconnect. A 403 (missing scope) is NOT a rejection: the
// credential is valid, it just lacks a grant.
func ClassifyCredentialError(status int, body []byte, err error) error {
	if status == http.StatusUnauthorized || credentialErrorBody(body) {
		return execution.RejectCredential(err)
	}
	return err
}

func credentialErrorBody(body []byte) bool {
	var envelope errorEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return false
	}
	return tokenErrorCodes[envelope.Error.Code]
}
