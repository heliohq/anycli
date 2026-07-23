package tiktok

import (
	"fmt"
	"net/http"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// credentialRejectionCodes are the TikTok in-envelope error codes that mean the
// user access token itself is no longer usable, so the host should invalidate
// the stored credential rather than retry.
var credentialRejectionCodes = map[string]bool{
	"access_token_invalid": true,
	"access_token_expired": true,
}

// newHTTPError renders a non-2xx TikTok response and classifies a 401 as an
// explicit credential rejection.
func newHTTPError(status int, body []byte, token string) error {
	err := fmt.Errorf("tiktok API error (HTTP %d %s): %s%s",
		status, http.StatusText(status), redact(string(body), token), httpHint(status))
	if status == http.StatusUnauthorized {
		return execution.RejectCredential(err)
	}
	return err
}

// newEnvelopeError renders a business error carried in the `error` object of a
// 2xx response and classifies token-invalid codes as credential rejections.
func newEnvelopeError(status int, apiErr apiErrorBody, token string) error {
	_ = status
	err := fmt.Errorf("tiktok API error (%s): %s", apiErr.Code, redact(apiErr.Message, token))
	if credentialRejectionCodes[apiErr.Code] {
		return execution.RejectCredential(err)
	}
	return err
}

func httpHint(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "; access token is invalid or expired — reconnect TikTok"
	case http.StatusForbidden:
		return "; token may lack the required scope or the app is not audited for this action"
	case http.StatusTooManyRequests:
		return "; rate limit exceeded — retry after the provider reset window"
	}
	return ""
}
