package tools

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// osFileCalls are the os functions that open a path the caller named. Reaching
// one directly bypasses Config.FS, which is the seam a host uses to decide what
// "local" means when AnyCLI runs somewhere other than the machine of the person
// who typed the command.
var osFileCalls = regexp.MustCompile(`\bos\.(ReadFile|WriteFile|Open|OpenFile|Create|CreateTemp|Remove|RemoveAll|Rename|MkdirAll|Mkdir)\b`)

// TestServiceFileAccessGoesThroughTheSeam mechanically enforces the filesystem
// contract over every registered service: a package that touches a local file
// must route it through execution.FileSystem and expose `FS` for injection.
//
// A failure here fails the build; fix the offending tool package, never this
// test. Either replace the os call with the execution helper, or — if the path
// is genuinely not caller-named — say so at the call site with a
// `//anycli:oshost` comment on the same line.
func TestServiceFileAccessGoesThroughTheSeam(t *testing.T) {
	names := ServiceNames()
	if len(names) == 0 {
		t.Fatal("no built-in service tools registered — registry seam broken")
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			svc, err := GetService(name)
			if err != nil {
				t.Fatalf("get service: %v", err)
			}
			directory := packageDirectory(t, svc)
			offenders := directOSFileCalls(t, directory)
			field, hasField := reflect.ValueOf(svc).Elem().Type().FieldByName(fsFieldName)

			if len(offenders) > 0 {
				t.Errorf("%s reaches the host filesystem directly:\n  %s",
					name, strings.Join(offenders, "\n  "))
			}
			if !hasField {
				return
			}
			if !fileSystemType.AssignableTo(field.Type) {
				t.Fatalf("%s has an FS field of type %s; it must accept execution.FileSystem", name, field.Type)
			}
			if got := WithFS(svc, stubFS{}); got == svc {
				t.Fatalf("%s was returned unchanged by WithFS — the FS field is not injectable", name)
			}
		})
	}
}

type stubFS struct{ execution.FileSystem }

func packageDirectory(t *testing.T, svc Service) string {
	t.Helper()
	path := reflect.TypeOf(svc).Elem().PkgPath()
	index := strings.Index(path, "/internal/tools/")
	if index < 0 {
		t.Fatalf("service package %q is not under internal/tools", path)
	}
	return filepath.Join(".", path[index+len("/internal/tools/"):])
}

func directOSFileCalls(t *testing.T, directory string) []string {
	t.Helper()
	entries, err := os.ReadDir(directory) //anycli:oshost — the test reads its own tree
	if err != nil {
		t.Fatalf("read %s: %v", directory, err)
	}
	var offenders []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(directory, name)
		data, err := os.ReadFile(path) //anycli:oshost — the test reads its own tree
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for number, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, "//anycli:oshost") {
				continue
			}
			if match := osFileCalls.FindString(line); match != "" {
				offenders = append(offenders, filepath.Join(path)+":"+itoa(number+1)+" "+match)
			}
		}
	}
	return offenders
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var digits []byte
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}
