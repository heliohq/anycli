// Package execution defines the outcome shared by built-in tool services and
// the AnyCLI execution engine. It is a leaf package so service implementations
// can report outcomes without importing the parent service registry.
package execution

import "errors"

// Result describes one built-in service invocation. CredentialRejected is true
// only when the provider explicitly rejects the resolved credential; ordinary
// command, scope/permission, rate-limit, transport, and provider failures leave it
// false so the engine does not invalidate a credential that may still be valid.
type Result struct {
	ExitCode           int
	CredentialRejected bool
}

type credentialRejectedError struct {
	cause error
}

func (e *credentialRejectedError) Error() string {
	return e.cause.Error()
}

func (e *credentialRejectedError) Unwrap() error {
	return e.cause
}

// RejectCredential marks a provider error as an explicit rejection of the
// resolved credential without changing the error text or hiding its cause.
func RejectCredential(err error) error {
	if err == nil || IsCredentialRejected(err) {
		return err
	}
	return &credentialRejectedError{cause: err}
}

// IsCredentialRejected reports whether a provider explicitly rejected the
// resolved credential.
func IsCredentialRejected(err error) bool {
	var rejected *credentialRejectedError
	return errors.As(err, &rejected)
}

// Failure converts a command error to the standard non-zero service result.
func Failure(err error) Result {
	return Result{ExitCode: 1, CredentialRejected: IsCredentialRejected(err)}
}
