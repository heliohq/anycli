package metaads

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// oauthExceptionCode is Graph's error.code for an invalid or expired access
// token (OAuthException). Meta issues no refresh grant, so this is the signal
// the assistant must re-consent rather than retry.
const oauthExceptionCode = 190

// graphErrorEnvelope is the standard Graph API error shape:
// {"error":{"message,type,code,error_subcode,fbtrace_id}}.
type graphErrorEnvelope struct {
	Error struct {
		Message      string `json:"message"`
		Type         string `json:"type"`
		Code         int    `json:"code"`
		ErrorSubcode int    `json:"error_subcode"`
		FBTraceID    string `json:"fbtrace_id"`
	} `json:"error"`
}

// newAPIError renders a Graph API failure with status, provider body, and a
// safe hint, redacting the bearer token. code:190 (OAuthException) is
// classified as a credential rejection so the host prompts a reconnect.
func newAPIError(status int, body []byte, token string) error {
	raw := strings.TrimSpace(string(body))
	if raw == "" {
		raw = "empty response body"
	}
	if token != "" {
		raw = strings.ReplaceAll(raw, token, "[REDACTED]")
	}
	if len(raw) > maxErrorBodyBytes {
		raw = raw[:maxErrorBodyBytes] + "…"
	}

	var envelope graphErrorEnvelope
	_ = json.Unmarshal(body, &envelope)

	hint := ""
	switch {
	case envelope.Error.Code == oauthExceptionCode || status == http.StatusUnauthorized:
		hint = "; access token is invalid or expired — reconnect Meta Ads"
	case status == http.StatusForbidden:
		hint = "; token may lack the required permission or ad-account access"
	case status == http.StatusTooManyRequests || envelope.Error.Code == 17 || envelope.Error.Code == 4:
		hint = "; rate/throttling limit reached — retry after the provider reset window"
	}

	err := fmt.Errorf("meta-ads API error (HTTP %d %s): %s%s", status, http.StatusText(status), raw, hint)
	if envelope.Error.Code == oauthExceptionCode || status == http.StatusUnauthorized {
		return execution.RejectCredential(err)
	}
	return err
}
