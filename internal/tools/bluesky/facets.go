package bluesky

import (
	"regexp"
	"strings"
)

// facet is one app.bsky.richtext.facet entry: a byte range plus one feature.
// Byte offsets index into the UTF-8 encoding of the post text (Go strings are
// already UTF-8, so strings.Index yields correct byte offsets directly).
type facet struct {
	Index    facetIndex     `json:"index"`
	Features []facetFeature `json:"features"`
}

type facetIndex struct {
	ByteStart int `json:"byteStart"`
	ByteEnd   int `json:"byteEnd"`
}

// facetFeature carries the discriminated $type plus exactly one populated
// payload field (uri for links, did for mentions, tag for hashtags).
type facetFeature struct {
	Type string `json:"$type"`
	URI  string `json:"uri,omitempty"`
	DID  string `json:"did,omitempty"`
	Tag  string `json:"tag,omitempty"`
}

const (
	facetLink    = "app.bsky.richtext.facet#link"
	facetMention = "app.bsky.richtext.facet#mention"
	facetTag     = "app.bsky.richtext.facet#tag"
)

var (
	// urlPattern matches bare http(s) URLs; trailing punctuation is trimmed
	// afterward so a sentence-final URL keeps a clean range.
	urlPattern = regexp.MustCompile(`https?://[^\s]+`)
	// tagPattern matches a hashtag preceded by start-of-text or whitespace.
	tagPattern = regexp.MustCompile(`(^|\s)(#[^\s#]+)`)
	// mentionPattern matches an @handle (a dotted domain-like handle).
	mentionPattern = regexp.MustCompile(`(^|\s)(@[a-zA-Z0-9][a-zA-Z0-9.-]+)`)
)

const trailingPunct = ".,;:!?)]}\"'"

// detectFacets computes rich-text facets from plain text. Links and hashtags
// are resolved purely from the text; mentions call resolveMention (best-effort,
// skipped on failure) so a post never fails because a handle could not resolve.
// resolveMention may be nil to skip mentions entirely.
func detectFacets(text string, resolveMention func(handle string) (did string, ok bool)) []facet {
	var facets []facet
	facets = append(facets, linkFacets(text)...)
	facets = append(facets, tagFacets(text)...)
	if resolveMention != nil {
		facets = append(facets, mentionFacets(text, resolveMention)...)
	}
	return facets
}

func linkFacets(text string) []facet {
	var out []facet
	for _, loc := range urlPattern.FindAllStringIndex(text, -1) {
		start, end := loc[0], loc[1]
		uri := text[start:end]
		trimmed := strings.TrimRight(uri, trailingPunct)
		end -= len(uri) - len(trimmed)
		if end <= start {
			continue
		}
		out = append(out, facet{
			Index:    facetIndex{ByteStart: start, ByteEnd: end},
			Features: []facetFeature{{Type: facetLink, URI: text[start:end]}},
		})
	}
	return out
}

func tagFacets(text string) []facet {
	var out []facet
	for _, loc := range tagPattern.FindAllStringSubmatchIndex(text, -1) {
		// loc[4]:loc[5] is the second capture group (the #tag token).
		start, end := loc[4], loc[5]
		token := text[start:end]
		trimmed := strings.TrimRight(token, trailingPunct)
		end -= len(token) - len(trimmed)
		tag := strings.TrimPrefix(text[start:end], "#")
		if tag == "" {
			continue
		}
		out = append(out, facet{
			Index:    facetIndex{ByteStart: start, ByteEnd: end},
			Features: []facetFeature{{Type: facetTag, Tag: tag}},
		})
	}
	return out
}

func mentionFacets(text string, resolve func(handle string) (string, bool)) []facet {
	var out []facet
	for _, loc := range mentionPattern.FindAllStringSubmatchIndex(text, -1) {
		start, end := loc[4], loc[5]
		token := text[start:end]
		trimmed := strings.TrimRight(token, trailingPunct)
		end -= len(token) - len(trimmed)
		handle := strings.TrimPrefix(text[start:end], "@")
		if handle == "" {
			continue
		}
		did, ok := resolve(handle)
		if !ok || did == "" {
			continue
		}
		out = append(out, facet{
			Index:    facetIndex{ByteStart: start, ByteEnd: end},
			Features: []facetFeature{{Type: facetMention, DID: did}},
		})
	}
	return out
}
