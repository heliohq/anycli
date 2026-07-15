package figma

import (
	"strings"
	"testing"
)

func TestParseFigmaURL(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		fileKey  string
		nodeID   string
		fileType string
	}{
		{name: "design", raw: "https://www.figma.com/design/AbC123/Product?node-id=12-34", fileKey: "AbC123", nodeID: "12:34", fileType: "design"},
		{name: "legacy file", raw: "https://figma.com/file/Key456/Legacy?node-id=1%3A2", fileKey: "Key456", nodeID: "1:2", fileType: "file"},
		{name: "figjam", raw: "https://www.figma.com/board/Jam789/Workshop", fileKey: "Jam789", fileType: "board"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			locator, err := parseFigmaURL(tc.raw)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if locator.FileKey != tc.fileKey || locator.NodeIDs != tc.nodeID || locator.FileType != tc.fileType {
				t.Errorf("locator = %+v", locator)
			}
		})
	}
}

func TestParseFigmaURLRejectsUntrustedOrIncompleteURLs(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{raw: "https://figma.com.attacker.example/design/key/name", want: "must be on figma.com"},
		{raw: "https://www.figma.com/community/file/123", want: "supported by PAT REST"},
		{raw: "https://www.figma.com/slides/Slide123/Deck", want: "supported by PAT REST"},
		{raw: "https://www.figma.com/make/Make123/App", want: "supported by PAT REST"},
		{raw: "not a url", want: "must be an absolute HTTPS URL"},
	}
	for _, tc := range cases {
		_, err := parseFigmaURL(tc.raw)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("parse %q error = %v, want %q", tc.raw, err, tc.want)
		}
	}
}

func TestLocatorOptionsResolveURLAndExplicitInput(t *testing.T) {
	cases := []struct {
		name    string
		opts    locatorOptions
		wantKey string
		wantIDs string
		wantErr string
	}{
		{name: "URL node", opts: locatorOptions{URL: "https://figma.com/design/abc/Test?node-id=1-2"}, wantKey: "abc", wantIDs: "1:2"},
		{name: "explicit overrides URL node", opts: locatorOptions{URL: "https://figma.com/design/abc/Test?node-id=1-2", NodeIDs: "3:4,5:6"}, wantKey: "abc", wantIDs: "3:4,5:6"},
		{name: "explicit", opts: locatorOptions{FileKey: "abc", NodeIDs: "1:2"}, wantKey: "abc", wantIDs: "1:2"},
		{name: "two sources", opts: locatorOptions{URL: "https://figma.com/design/abc/Test", FileKey: "def"}, wantErr: "mutually exclusive"},
		{name: "no source", opts: locatorOptions{}, wantErr: "one of --url or --file-key is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			locator, err := tc.opts.resolve()
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if locator.FileKey != tc.wantKey || locator.NodeIDs != tc.wantIDs {
				t.Errorf("locator = %+v", locator)
			}
		})
	}
}
