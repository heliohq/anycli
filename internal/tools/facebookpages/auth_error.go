package facebookpages

import (
	"net/http"

	"github.com/heliohq/anycli/internal/tools/execution"
)

const (
	// codeOAuthException (Graph error code 190) is returned for an expired,
	// revoked, or otherwise invalid access token. It is the "reconnect needed"
	// signal — classified as a credential rejection so the engine prompts
	// re-auth rather than retrying blindly.
	codeOAuthException = 190
	// codePermission (Graph error code 200) is returned when the token lacks a
	// required Page permission/task or scope. It is a permission failure, NOT a
	// credential rejection: the credential is valid, the grant is insufficient,
	// so the engine must not invalidate the connection.
	codePermission = 200
)

// classifyCredentialError marks an expired/invalid-token failure (HTTP 401 or
// Graph code 190) as an explicit credential rejection. A permission error
// (code 200) is deliberately left unclassified so a valid connection is not
// invalidated over an insufficient-scope call.
func classifyCredentialError(status int, ge graphError, err error) error {
	if status == http.StatusUnauthorized || ge.Code == codeOAuthException {
		return execution.RejectCredential(err)
	}
	return err
}
