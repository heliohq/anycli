package reddit

import (
	"net/http"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// classifyRedditCredentialError marks an HTTP 401 as an explicit credential
// rejection so the engine invalidates the stored token. 403 (scope/permission)
// and 429 (rate limit) are ordinary failures — the token may still be valid.
func classifyRedditCredentialError(status int, err error) error {
	if status == http.StatusUnauthorized {
		return execution.RejectCredential(err)
	}
	return err
}
