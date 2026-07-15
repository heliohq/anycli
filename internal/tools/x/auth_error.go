package x

import (
	"encoding/json"
	"net/http"

	"github.com/heliohq/anycli/internal/tools/execution"
)

const invalidOrExpiredTokenCode = 89

func classifyXCredentialError(status int, body []byte, err error) error {
	if status == http.StatusUnauthorized || xInvalidTokenError(body) {
		return execution.RejectCredential(err)
	}
	return err
}

func xInvalidTokenError(body []byte) bool {
	var envelope struct {
		Errors []struct {
			Code int `json:"code"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return false
	}
	for _, apiErr := range envelope.Errors {
		if apiErr.Code == invalidOrExpiredTokenCode {
			return true
		}
	}
	return false
}
