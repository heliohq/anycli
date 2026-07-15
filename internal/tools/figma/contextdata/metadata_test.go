package contextdata

import (
	"strings"
	"testing"
)

func TestExtractMetadataFromFileAndNodeResponses(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{name: "file", raw: `{"name":"Product","document":{"id":"0:0","name":"Document","type":"DOCUMENT","children":[{"id":"1:2","name":"Hero","type":"FRAME","layoutMode":"VERTICAL","absoluteBoundingBox":{"x":10,"y":20,"width":300,"height":200},"children":[{"id":"2:3","name":"Title","type":"TEXT","characters":"Hello"}]}]}}`},
		{name: "nodes", raw: `{"nodes":{"1:2":{"document":{"id":"1:2","name":"Hero","type":"FRAME","children":[{"id":"2:3","name":"Title","type":"TEXT","characters":"Hello"}]}}}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			metadata, err := ExtractMetadata([]byte(tc.raw), 10)
			if err != nil {
				t.Fatalf("extract: %v", err)
			}
			if metadata.NodeCount == 0 || len(metadata.Roots) != 1 {
				t.Fatalf("metadata = %+v", metadata)
			}
			root := metadata.Roots[0]
			if root.ID == "" || root.Name == "" || root.Type == "" {
				t.Errorf("root = %+v", root)
			}
		})
	}
}

func TestExtractMetadataTruncatesDeterministically(t *testing.T) {
	raw := []byte(`{"document":{"id":"0:0","name":"Document","type":"DOCUMENT","children":[{"id":"1:1","name":"One","type":"FRAME"},{"id":"1:2","name":"Two","type":"FRAME"}]}}`)
	metadata, err := ExtractMetadata(raw, 2)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.NodeCount != 2 || !metadata.Truncated {
		t.Errorf("metadata = %+v", metadata)
	}
	if len(metadata.Roots) != 1 || len(metadata.Roots[0].Children) != 1 || metadata.Roots[0].Children[0].ID != "1:1" {
		t.Errorf("roots = %+v", metadata.Roots)
	}
}

func TestExtractMetadataRejectsInvalidResponsesAndLimits(t *testing.T) {
	cases := []struct {
		raw   string
		limit int
		want  string
	}{
		{raw: `{`, limit: 10, want: "decode Figma document"},
		{raw: `{}`, limit: 10, want: "contains no document nodes"},
		{raw: `{"document":{"id":"0:0"}}`, limit: 0, want: "max nodes must be positive"},
	}
	for _, tc := range cases {
		_, err := ExtractMetadata([]byte(tc.raw), tc.limit)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("error = %v, want %q", err, tc.want)
		}
	}
}
