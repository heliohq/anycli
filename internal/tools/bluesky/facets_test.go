package bluesky

import "testing"

func TestDetectFacetsByteOffsets(t *testing.T) {
	facets := detectFacets("Check https://example.com and #golang", nil)
	if len(facets) != 2 {
		t.Fatalf("facets = %+v", facets)
	}
	if facets[0].Index != (facetIndex{ByteStart: 6, ByteEnd: 25}) || facets[0].Features[0].URI != "https://example.com" {
		t.Fatalf("link facet = %+v", facets[0])
	}
	if facets[1].Index != (facetIndex{ByteStart: 30, ByteEnd: 37}) || facets[1].Features[0].Tag != "golang" {
		t.Fatalf("tag facet = %+v", facets[1])
	}
}

func TestDetectFacetsTrimsTrailingPunctuation(t *testing.T) {
	// The period must not be swallowed into the link range.
	facets := detectFacets("see https://example.com.", nil)
	if len(facets) != 1 {
		t.Fatalf("facets = %+v", facets)
	}
	if facets[0].Features[0].URI != "https://example.com" {
		t.Fatalf("uri = %q", facets[0].Features[0].URI)
	}
	if facets[0].Index.ByteEnd != 23 { // "see " = 4, URL 19 → 4+19 = 23
		t.Fatalf("byteEnd = %d, want 23", facets[0].Index.ByteEnd)
	}
}

func TestDetectFacetsUsesUTF8ByteOffsets(t *testing.T) {
	// "café " is 6 bytes (é is 2 bytes), so the link starts at byte 6, proving
	// offsets are byte-based (not rune-based).
	facets := detectFacets("café https://x.io", nil)
	if len(facets) != 1 {
		t.Fatalf("facets = %+v", facets)
	}
	if facets[0].Index.ByteStart != 6 {
		t.Fatalf("byteStart = %d, want 6", facets[0].Index.ByteStart)
	}
}

func TestDetectFacetsResolvesMentions(t *testing.T) {
	resolve := func(handle string) (string, bool) {
		if handle == "alice.bsky.social" {
			return "did:plc:alice", true
		}
		return "", false
	}
	facets := detectFacets("hi @alice.bsky.social and @ghost.example", resolve)
	// alice resolves → one mention facet; ghost does not → skipped.
	var mentions int
	for _, f := range facets {
		if f.Features[0].Type == facetMention {
			mentions++
			if f.Features[0].DID != "did:plc:alice" {
				t.Fatalf("mention did = %q", f.Features[0].DID)
			}
		}
	}
	if mentions != 1 {
		t.Fatalf("mentions = %d, want 1 (unresolvable handle skipped)", mentions)
	}
}

func TestParseATURI(t *testing.T) {
	uri, err := parseATURI("at://did:plc:alice/app.bsky.feed.post/abc")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if uri.Authority != "did:plc:alice" || uri.Collection != "app.bsky.feed.post" || uri.RKey != "abc" {
		t.Fatalf("parsed = %+v", uri)
	}
	for _, bad := range []string{"https://x", "at://only", "at://a/b", "at:///b/c"} {
		if _, err := parseATURI(bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}
