package tools

import (
	"reflect"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// fsFieldName is the conventional exported filesystem field every built-in
// service that touches a local file carries: `FS execution.FileSystem`; nil =
// the os package. WithFS keys off it structurally, the same way WithHTTPClient
// keys off HC, so a new service picks up engine-level filesystem injection by
// following the convention — no per-service registration.
const fsFieldName = "FS"

var fileSystemType = reflect.TypeFor[execution.FileSystem]()

// WithFS returns a shallow copy of svc with its exported FS field set to fs,
// leaving the registered singleton untouched (safe under concurrent engines).
// A service without an FS field (one that reads and writes no local files) is
// returned unchanged. fs must be non-nil.
func WithFS(svc Service, fs execution.FileSystem) Service {
	if fs == nil {
		return svc
	}
	v := reflect.ValueOf(svc)
	if v.Kind() != reflect.Pointer || v.Elem().Kind() != reflect.Struct {
		return svc
	}
	field, ok := v.Elem().Type().FieldByName(fsFieldName)
	if !ok || !fileSystemType.AssignableTo(field.Type) {
		return svc
	}
	cp := reflect.New(v.Elem().Type())
	cp.Elem().Set(v.Elem())
	cp.Elem().FieldByName(fsFieldName).Set(reflect.ValueOf(fs))
	return cp.Interface().(Service)
}
