package klaviyo

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, bad enum value, invalid JSON, or a missing id. It maps to exit
// code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Klaviyo non-2xx response, a transport
// failure, or a decode failure. It maps to exit code 1 and kind "api". status
// is the HTTP status (0 for transport/network failures). It wraps the
// underlying cause so errors.As for the credential-rejection classification
// still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }
