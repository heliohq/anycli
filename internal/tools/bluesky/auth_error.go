package bluesky

import (
	"errors"
	"net/http"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// Named atproto XRPC error codes we branch on. See
// com.atproto.server.* error responses.
const (
	errExpiredToken            = "ExpiredToken"
	errInvalidToken            = "InvalidToken"
	errAuthenticationRequired  = "AuthenticationRequired"
	errAuthFactorTokenRequired = "AuthFactorTokenRequired"
)

// classifyCredentialError marks the provider error as an explicit credential
// rejection when the identifier/app-password pair or the derived session is
// itself invalid — never for transient token expiry, rate limits, or server
// faults, which must not invalidate a still-valid stored credential.
func classifyCredentialError(status int, name string, err error) error {
	switch {
	case name == errInvalidToken,
		name == errAuthenticationRequired,
		name == errAuthFactorTokenRequired,
		status == http.StatusUnauthorized:
		return execution.RejectCredential(err)
	default:
		return err
	}
}

// isExpiredToken reports whether the error is the transient ExpiredToken
// signal, which the session recovers from by re-establishing once.
func isExpiredToken(err error) bool {
	var provErr *providerError
	return errors.As(err, &provErr) && provErr.Name == errExpiredToken
}
