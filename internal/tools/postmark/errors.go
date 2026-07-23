package postmark

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, bad enum value, or invalid JSON. It maps to exit code 2 and
// kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Postmark non-2xx response, a 2xx body
// carrying a non-zero ErrorCode, or a transport failure. It maps to exit code 1
// and kind "api". status is the HTTP status (0 for transport/network failures);
// errorCode is Postmark's application ErrorCode (0 when not applicable). It
// wraps the underlying cause so errors.As for *credentialRejectedError still
// resolves through it.
type apiError struct {
	msg       string
	status    int
	errorCode int
	err       error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// hasJSONArg reports whether the raw args carry the --json global flag, used to
// pick the error format before cobra has parsed flags (e.g. the pre-parse
// missing-token check).
func hasJSONArg(args []string) bool {
	for _, a := range args {
		if a == "--json" || a == "--json=true" {
			return true
		}
	}
	return false
}

// renderError writes err to stderr. Under --json the shape is
// {"error":{"message":…,"kind":"usage|api","status":<HTTP>,"error_code":<int>}}
// where status/error_code are omitted when not applicable.
func (s *Service) renderError(jsonMode bool, err error) {
	if !jsonMode {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	payload := map[string]any{"message": err.Error(), "kind": "usage"}
	if apiErr, ok := err.(*apiError); ok {
		payload["kind"] = "api"
		if apiErr.status != 0 {
			payload["status"] = apiErr.status
		}
		if apiErr.errorCode != 0 {
			payload["error_code"] = apiErr.errorCode
		}
	}
	b, mErr := json.Marshal(map[string]any{"error": payload})
	if mErr != nil {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	fmt.Fprintln(s.stderr(), string(b))
}

// usagef builds a usageError from a format string (exit 2).
func usagef(format string, args ...any) error {
	return &usageError{msg: fmt.Sprintf(format, args...)}
}

// requireArgs is a cobra Args validator that reports missing positional
// arguments as a usageError (exit 2) rather than cobra's default error text.
func requireArgs(n int, usage string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) != n {
			return usagef("postmark: %s", usage)
		}
		return nil
	}
}
