package later

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// newTokenError classifies a non-2xx from POST /oauth/token. The exchanged
// pair IS the credential, so any 400/401/403 (the provider rejecting the
// clientId/clientSecret) marks the credential rejected; other statuses are
// ordinary transient failures.
func newTokenError(status int, body []byte) error {
	err := fmt.Errorf("later /oauth/token error (HTTP %d %s): %s", status, http.StatusText(status), snippet(body))
	switch status {
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden:
		return execution.RejectCredential(err)
	}
	return err
}

// newAPIError classifies a non-2xx from a data endpoint. A 401 that survives a
// re-mint means the credential no longer authorizes; 403 is a
// scope/entitlement problem that does not invalidate the credential.
func newAPIError(status int, body []byte) error {
	err := fmt.Errorf("later API error (HTTP %d %s): %s", status, http.StatusText(status), snippet(body))
	if status == http.StatusUnauthorized {
		return execution.RejectCredential(err)
	}
	return err
}

// snippet trims and bounds an error body for a single-line message.
func snippet(body []byte) string {
	raw := strings.TrimSpace(string(body))
	if raw == "" {
		return "empty response body"
	}
	if len(raw) > maxErrorBodyBytes {
		raw = raw[:maxErrorBodyBytes] + "…"
	}
	return raw
}
