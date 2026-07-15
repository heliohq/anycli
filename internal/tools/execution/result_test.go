package execution

import (
	"errors"
	"testing"
)

func TestRejectCredentialPreservesCauseAndClassifiesFailure(t *testing.T) {
	cause := errors.New("provider error text")
	err := RejectCredential(cause)

	if err.Error() != cause.Error() {
		t.Errorf("error = %q, want %q", err, cause)
	}
	if !errors.Is(err, cause) {
		t.Fatal("rejected error does not preserve its cause")
	}
	if !IsCredentialRejected(err) {
		t.Fatal("rejected error was not classified")
	}

	result := Failure(err)
	if result.ExitCode != 1 || !result.CredentialRejected {
		t.Errorf("Failure = %+v, want exit 1 with credential rejection", result)
	}
}

func TestFailureDoesNotRejectCredentialForOrdinaryError(t *testing.T) {
	result := Failure(errors.New("permission denied"))
	if result.ExitCode != 1 || result.CredentialRejected {
		t.Errorf("Failure = %+v, want exit 1 without credential rejection", result)
	}
}
