package notion

import (
	"encoding/json"
	"net/http"

	"github.com/heliohq/anycli/internal/tools/execution"
)

func classifyNotionCredentialError(status int, body []byte, err error) error {
	if status == http.StatusUnauthorized || notionErrorCode(body) == "unauthorized" {
		return execution.RejectCredential(err)
	}
	return err
}

func notionErrorCode(body []byte) string {
	var envelope struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ""
	}
	return envelope.Code
}
