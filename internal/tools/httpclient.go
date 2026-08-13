package tools

import (
	"net/http"
	"reflect"
)

// hcFieldName is the conventional exported HTTP-client field every built-in
// service that talks to a provider carries: `HC *http.Client` (or an
// interface *http.Client satisfies, e.g. a package-local httpDoer); nil =
// http.DefaultClient. WithHTTPClient keys off it structurally, so a new
// service picks up engine-level HTTP injection by following the convention —
// no per-service registration.
const hcFieldName = "HC"

var httpClientType = reflect.TypeFor[*http.Client]()

// WithHTTPClient returns a shallow copy of svc with its exported HC field set
// to hc, leaving the registered singleton untouched (safe under concurrent
// engines). A service without an HC field *http.Client can populate (a
// service that makes no provider HTTP calls, e.g. mongodb or gate-probe) is
// returned unchanged. hc must be non-nil.
func WithHTTPClient(svc Service, hc *http.Client) Service {
	if hc == nil {
		return svc
	}
	v := reflect.ValueOf(svc)
	if v.Kind() != reflect.Pointer || v.Elem().Kind() != reflect.Struct {
		return svc
	}
	field, ok := v.Elem().Type().FieldByName(hcFieldName)
	if !ok || !httpClientType.AssignableTo(field.Type) {
		return svc
	}
	cp := reflect.New(v.Elem().Type())
	cp.Elem().Set(v.Elem())
	cp.Elem().FieldByName(hcFieldName).Set(reflect.ValueOf(hc))
	return cp.Interface().(Service)
}
