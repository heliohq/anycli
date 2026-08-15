package tools

import (
	"net/http"
	"reflect"
	"testing"
)

// noProviderHTTPServices are the registered services that make no outbound
// HTTP calls at all and therefore legitimately carry no HC field. Keep this
// set closed: a new service that makes any outbound HTTP — provider calls or
// lazy binary downloads — must expose `HC *http.Client` so engine-level HTTP
// injection (anycli.Config.HTTPClient) covers it; add it here only if it
// genuinely does no HTTP. mongodb is NOT exempt: its wire protocol is not
// HTTP, but its lazy mongosh download is.
var noProviderHTTPServices = map[string]bool{
	"gate-probe": true, // credential-free local echo harness; no outbound HTTP
}

// TestWithHTTPClientCoversEveryRegisteredService mechanically enforces the
// injection contract over the full registry: WithHTTPClient must return a
// copy with HC set for every service, except the closed no-HTTP set, and the
// registered singleton must never be mutated.
func TestWithHTTPClientCoversEveryRegisteredService(t *testing.T) {
	hc := &http.Client{}
	for _, name := range ServiceNames() {
		t.Run(name, func(t *testing.T) {
			svc, err := GetService(name)
			if err != nil {
				t.Fatalf("get service: %v", err)
			}
			got := WithHTTPClient(svc, hc)
			if noProviderHTTPServices[name] {
				if got != svc {
					t.Fatalf("service %q is declared no-HTTP but WithHTTPClient returned a copy — remove it from noProviderHTTPServices", name)
				}
				if _, has := reflect.ValueOf(svc).Elem().Type().FieldByName(hcFieldName); has {
					t.Fatalf("service %q has an HC field but is listed in noProviderHTTPServices — remove it from the set", name)
				}
				return
			}
			if got == svc {
				t.Fatalf("service %q was returned unchanged — it must expose `HC *http.Client` for engine-level HTTP injection, or be added to noProviderHTTPServices if it truly makes no HTTP calls", name)
			}
			// HC may be typed *http.Client or a package-local interface it
			// satisfies; compare through the empty interface either way.
			if injected := reflect.ValueOf(got).Elem().FieldByName(hcFieldName).Interface(); injected != any(hc) {
				t.Fatalf("service %q copy has HC = %v, want the injected client", name, injected)
			}
			if orig := reflect.ValueOf(svc).Elem().FieldByName(hcFieldName); !orig.IsNil() {
				t.Fatalf("service %q registered singleton was mutated (HC = %v)", name, orig)
			}
		})
	}
}

// TestWithHTTPClientNilClientIsIdentity pins the nil-client contract: no copy,
// no mutation.
func TestWithHTTPClientNilClientIsIdentity(t *testing.T) {
	svc, err := GetService("gmail")
	if err != nil {
		t.Fatalf("get service: %v", err)
	}
	if got := WithHTTPClient(svc, nil); got != svc {
		t.Fatal("WithHTTPClient(svc, nil) must return svc unchanged")
	}
}
