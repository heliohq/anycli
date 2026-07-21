package attio

import (
	"encoding/json"
	"net/http"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// classifyAttioCredentialError marks a 401 (or an authentication_error /
// invalid_token body) as an explicit credential rejection so the host's
// token-refresh path can react, while every other non-2xx (403 scope, 404,
// 400 validation, 429 rate-limit) stays an ordinary failure that must not
// invalidate a still-valid token.
func classifyAttioCredentialError(status int, body []byte, err error) error {
	if status == http.StatusUnauthorized || isAuthErrorBody(body) {
		return execution.RejectCredential(err)
	}
	return err
}

// isAuthErrorBody reports whether Attio's error envelope names an
// authentication failure (type "authentication_error" or a token-related code).
func isAuthErrorBody(body []byte) bool {
	var e struct {
		Type string `json:"type"`
		Code string `json:"code"`
	}
	if err := json.Unmarshal(body, &e); err != nil {
		return false
	}
	if e.Type == "authentication_error" {
		return true
	}
	switch e.Code {
	case "invalid_token", "unauthorized", "authentication_required":
		return true
	}
	return false
}
