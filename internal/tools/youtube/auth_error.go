package youtube

import (
	"encoding/json"
	"net/http"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// errorEnvelope is the standard Google API JSON error shape. YouTube Data API
// v3 returns the same structure as the rest of the Google family.
type errorEnvelope struct {
	Error struct {
		Status string `json:"status"`
		Errors []struct {
			Reason string `json:"reason"`
		} `json:"errors"`
	} `json:"error"`
}

// classifyCredentialError marks only genuine authentication failures as a
// credential rejection so the engine invalidates the stored token. A 403
// (permission / insufficient scope / quota) leaves the credential intact — the
// token is valid, the grant or quota is not.
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
