package lusha

import (
	"net/http"
	"testing"
)

func TestAccountUsagePassthrough(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"credits":{"used":1500,"remaining":8500,"total":10000},"plan":{"category":"professional"}}`, &got)
	defer srv.Close()

	code, out, _ := run(t, srv, "account", "usage")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodGet || got.Path != "/account/usage" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	if len(got.Body) != 0 {
		t.Errorf("GET must have no body, got %s", got.Body)
	}
	// The usage object is passed through under data.
	data := decodeStdout(t, out)["data"].(map[string]any)
	credits := data["credits"].(map[string]any)
	if credits["remaining"].(float64) != 8500 {
		t.Errorf("credits = %v", credits)
	}
}
