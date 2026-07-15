package google

import (
	"encoding/json"
	"net/http"

	"github.com/heliohq/anycli/internal/tools/execution"
)

type googleErrorEnvelope struct {
	Error struct {
		Status string `json:"status"`
		Errors []struct {
			Reason string `json:"reason"`
		} `json:"errors"`
	} `json:"error"`
}

func classifyGoogleCredentialError(status int, body []byte, err error) error {
	if status == http.StatusUnauthorized || googleCredentialErrorBody(body) {
		return execution.RejectCredential(err)
	}
	return err
}

func googleCredentialErrorBody(body []byte) bool {
	var envelope googleErrorEnvelope
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
