package pennylane

import (
	"net/http"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// classifyPennylaneCredentialError marks a 401 as an explicit credential
// rejection so the engine can invalidate the stored token, while leaving 403
// (missing scope) and every other status as an ordinary failure that must not
// invalidate a credential that may still be valid.
func classifyPennylaneCredentialError(status int, err error) error {
	if status == http.StatusUnauthorized {
		return execution.RejectCredential(err)
	}
	return err
}
