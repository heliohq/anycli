package figma

import (
	"context"
	"strings"
	"testing"
)

func TestEmbeddedOperationCatalogMatchesPinnedFigmaSpec(t *testing.T) {
	catalog, err := loadOperationCatalog()
	if err != nil {
		t.Fatalf("load operation catalog: %v", err)
	}
	if catalog.Source != "figma/rest-api-spec@e854a2c2dff3ff8cb743e9a06575fbbf225faa33" {
		t.Errorf("source = %q", catalog.Source)
	}
	if len(catalog.Operations) != 50 {
		t.Fatalf("operation count = %d, want 50", len(catalog.Operations))
	}

	ids := map[string]struct{}{}
	patOperations := 0
	for _, operation := range catalog.Operations {
		if _, exists := ids[operation.ID]; exists {
			t.Errorf("duplicate operation ID %q", operation.ID)
		}
		ids[operation.ID] = struct{}{}
		if operation.PAT {
			patOperations++
		}
		if err := operation.validate(); err != nil {
			t.Errorf("operation %q: %v", operation.ID, err)
		}
	}
	if patOperations != 47 {
		t.Errorf("PAT operation count = %d, want 47", patOperations)
	}
}

func TestOperationResolveEscapesPathsAndSeparatesQueries(t *testing.T) {
	catalog, err := loadOperationCatalog()
	if err != nil {
		t.Fatal(err)
	}
	operation, ok := catalog.find("getFileNodes")
	if !ok {
		t.Fatal("getFileNodes missing")
	}
	path, query, err := operation.resolve([]string{
		"file_key=abc/branch",
		"ids=1:2,3:4",
		"plugin_data=one",
		"plugin_data=two",
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if path != "/v1/files/abc%2Fbranch/nodes" {
		t.Errorf("path = %q", path)
	}
	if query.Get("ids") != "1:2,3:4" {
		t.Errorf("ids = %q", query.Get("ids"))
	}
	if got := query["plugin_data"]; len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Errorf("plugin_data = %v", got)
	}
}

func TestOperationResolveRejectsInvalidParameters(t *testing.T) {
	catalog, err := loadOperationCatalog()
	if err != nil {
		t.Fatal(err)
	}
	operation, _ := catalog.find("getFileNodes")
	cases := []struct {
		name   string
		params []string
		want   string
	}{
		{name: "missing path", params: []string{"ids=1:2"}, want: "missing required parameter file_key"},
		{name: "unknown", params: []string{"file_key=abc", "ids=1:2", "secret=bad"}, want: "unknown parameter secret"},
		{name: "malformed", params: []string{"file_key"}, want: "--param must use key=value"},
		{name: "duplicate path", params: []string{"file_key=one", "file_key=two", "ids=1:2"}, want: "path parameter file_key may be set only once"},
		{name: "missing required query", params: []string{"file_key=abc"}, want: "missing required query parameter ids"},
		{name: "empty required query", params: []string{"file_key=abc", "ids="}, want: "missing required query parameter ids"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := operation.resolve(tc.params)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestMissingNamedOperationReturnsErrorWithoutPanicking(t *testing.T) {
	service := &Service{}
	command := service.newOperationCommand("figd_test_token", operationCommandSpec{
		Use:         "missing",
		Short:       "Missing catalog operation",
		OperationID: "operationThatDoesNotExist",
	})
	command.SetArgs(nil)
	if err := command.ExecuteContext(context.Background()); err == nil || !strings.Contains(err.Error(), "unknown Figma operation") {
		t.Fatalf("error = %v, want unknown-operation error", err)
	}
}
