package outreach

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// scopeErrorID is the JSON:API error id Outreach returns when the token lacks a
// required OAuth scope. Outreach returns it with HTTP 403.
const scopeErrorID = "unauthorizedOauthScope"

// classifyCredentialError marks a provider error as an explicit credential
// rejection when Outreach reports an invalid/expired token (HTTP 401) or an
// unauthorized-scope error (HTTP 403 with the documented error id). Other
// failures (rate limit, validation, server errors) leave the credential valid.
func classifyCredentialError(status int, body []byte, err error) error {
	if status == http.StatusUnauthorized || hasScopeError(body) {
		return execution.RejectCredential(err)
	}
	return err
}

// hasScopeError reports whether the JSON:API errors[] body carries the
// unauthorized-scope error id (case-insensitive).
func hasScopeError(body []byte) bool {
	var envelope struct {
		Errors []struct {
			ID string `json:"id"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return false
	}
	for _, e := range envelope.Errors {
		if strings.EqualFold(e.ID, scopeErrorID) {
			return true
		}
	}
	return false
}
