package acuity

import (
	"net/http"
	"testing"
)

func TestSimpleListEndpoints(t *testing.T) {
	cases := []struct {
		args []string
		path string
	}{
		{[]string{"type", "list"}, "/appointment-types"},
		{[]string{"calendar", "list"}, "/calendars"},
		{[]string{"form", "list"}, "/forms"},
		{[]string{"label", "list"}, "/labels"},
		{[]string{"me"}, "/me"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			var got capturedRequest
			srv := newServer(t, 200, `[]`, &got)
			defer srv.Close()

			code, stdout, _ := run(t, srv, tc.args...)
			if code != 0 {
				t.Fatalf("exit = %d, want 0", code)
			}
			if got.Method != http.MethodGet || got.Path != tc.path {
				t.Fatalf("request = %s %s, want GET %s", got.Method, got.Path, tc.path)
			}
			if stdout != "[]\n" {
				t.Errorf("stdout = %q, want passthrough", stdout)
			}
		})
	}
}
