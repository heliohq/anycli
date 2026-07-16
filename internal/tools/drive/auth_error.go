package drive

import (
	"encoding/json"
	"net/http"

	"github.com/heliohq/anycli/internal/tools/execution"
)

type errorEnvelope struct {
	Error struct {
		Status string `json:"status"`
		Errors []struct {
			Reason string `json:"reason"`
		} `json:"errors"`
	} `json:"error"`
}

// classifyCredentialError marks only genuine credential rejection (HTTP 401 or
// an explicit UNAUTHENTICATED / authError body) so the engine invalidates the
// stored token. A 403 PERMISSION_DENIED (missing scope) leaves the credential
// intact — the fix is reconnect, not re-mint.
func classifyCredentialError(status int, body []byte, err error) error {
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
	if envelope.Error.Status == "UNAUTHENTICATED" {
		return true
	}
	for _, detail := range envelope.Error.Errors {
		if detail.Reason == "authError" || detail.Reason == "invalidCredentials" {
			return true
		}
	}
	return false
}
