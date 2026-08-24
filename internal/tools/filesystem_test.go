package tools

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// osFileCalls are the os functions that touch a file. A tool package must not
// call any of them: execution.FileSystem is where local files go, and
// execution.OS is the implementation that reaches the real disk. Two paths
// would mean a caller-named path could escape the seam, and a behavior that
// exists only when a host declines to install one.
var osFileCalls = regexp.MustCompile(`\bos\.(ReadFile|WriteFile|Open|OpenFile|Create|CreateTemp|Remove|RemoveAll|Rename|MkdirAll|Mkdir|MkdirTemp|Stat|Lstat|Chmod|Chown|Link|Symlink|Truncate|ReadDir)\b`)

// scratchMarker exempts one line. It means: this path carries no caller input
// — it is scratch the function just made for itself — so routing it through
// the seam would give a host something to configure without giving a caller
// anything to reach. Every use must say why on the same line, and the count
// below pins how many exist so a new one is a deliberate edit rather than a
// habit.
const scratchMarker = "//anycli:scratch"

// expectedScratchSites is the whole of the exempt set:
//
//   - figma's response spool, which holds up to 1 GiB while the JSON is
//     validated and then re-read, in a directory no flag names;
//   - mongodb's scoped HOME, which exists to be handed to mongosh. That one is
//     a subprocess: it reads with its own system calls, so no Go interface
//     could cover it, and a host that cares declines to run mongodb at all;
//   - supabase's scoped HOME, which prevents the official CLI subprocess from
//     reading ambient credentials or persisting profile and telemetry state.
const expectedScratchSites = 6

func TestServiceFileAccessGoesThroughTheSeam(t *testing.T) {
	names := ServiceNames()
	if len(names) == 0 {
		t.Fatal("no built-in service tools registered — registry seam broken")
	}
	scratch := 0
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			svc, err := GetService(name)
			if err != nil {
				t.Fatalf("get service: %v", err)
			}
			directory := packageDirectory(t, svc)
			offenders, exempt := scanOSFileCalls(t, directory)
			scratch += exempt
			if len(offenders) > 0 {
				t.Errorf("%s reaches the host filesystem directly:\n  %s",
					name, strings.Join(offenders, "\n  "))
			}
			field, hasField := reflect.ValueOf(svc).Elem().Type().FieldByName(fsFieldName)
			if !hasField {
				return
			}
			if !fileSystemType.AssignableTo(field.Type) {
				t.Fatalf("%s has an FS field of type %s; it must accept execution.FileSystem", name, field.Type)
			}
			if got := WithFS(svc, execution.OS{}); got == svc {
				t.Fatalf("%s was returned unchanged by WithFS — the FS field is not injectable", name)
			}
		})
	}
	if scratch != expectedScratchSites {
		t.Errorf("%d %s sites, want %d — adding one is a decision, not a detail",
			scratch, scratchMarker, expectedScratchSites)
	}
}

func packageDirectory(t *testing.T, svc Service) string {
	t.Helper()
	path := reflect.TypeOf(svc).Elem().PkgPath()
	index := strings.Index(path, "/internal/tools/")
	if index < 0 {
		t.Fatalf("service package %q is not under internal/tools", path)
	}
	return filepath.Join(".", path[index+len("/internal/tools/"):])
}

// scanOSFileCalls returns the unmarked os file calls in a package's
// non-test sources, and how many marked ones it skipped.
func scanOSFileCalls(t *testing.T, directory string) ([]string, int) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read %s: %v", directory, err)
	}
	var offenders []string
	exempt := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(directory, name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for number, line := range strings.Split(string(data), "\n") {
			marked := strings.Contains(line, scratchMarker)
			// A comment naming one of these is prose, not a call. Strip it
			// before matching so a line explaining why os.Link was dropped does
			// not read as a use of it.
			if comment := strings.Index(line, "//"); comment >= 0 {
				line = line[:comment]
			}
			match := osFileCalls.FindString(line)
			if match == "" {
				continue
			}
			if marked {
				exempt++
				continue
			}
			offenders = append(offenders, path+":"+strconv.Itoa(number+1)+" "+match)
		}
	}
	return offenders, exempt
}
