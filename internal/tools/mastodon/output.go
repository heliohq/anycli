package mastodon

import (
	"encoding/json"
	"html"
	"regexp"
	"strings"
)

// rawStatus is the subset of a Mastodon Status object the service reads. The
// full object is much larger; only these fields feed the provider-neutral
// output shapes.
type rawStatus struct {
	ID               string      `json:"id"`
	URL              string      `json:"url"`
	URI              string      `json:"uri"`
	Content          string      `json:"content"`
	CreatedAt        string      `json:"created_at"`
	Visibility       string      `json:"visibility"`
	RepliesCount     int         `json:"replies_count"`
	ReblogsCount     int         `json:"reblogs_count"`
	FavouritesCount  int         `json:"favourites_count"`
	Account          rawAccount  `json:"account"`
	MediaAttachments []anyObject `json:"media_attachments"`
}

// rawAccount is the subset of a Mastodon Account object the service reads.
type rawAccount struct {
	ID             string `json:"id"`
	Username       string `json:"username"`
	Acct           string `json:"acct"`
	DisplayName    string `json:"display_name"`
	URL            string `json:"url"`
	Note           string `json:"note"`
	FollowersCount int    `json:"followers_count"`
	FollowingCount int    `json:"following_count"`
	StatusesCount  int    `json:"statuses_count"`
}

type anyObject = map[string]any

// statusSummary is the provider-neutral post shape emitted for timelines,
// search results, and account posts. content_text is the HTML content stripped
// to plain text so the AI reads posts without parsing markup.
type statusSummary struct {
	ID              string     `json:"id"`
	URL             string     `json:"url"`
	Account         accountRef `json:"account"`
	ContentText     string     `json:"content_text"`
	CreatedAt       string     `json:"created_at"`
	Visibility      string     `json:"visibility,omitempty"`
	RepliesCount    int        `json:"replies_count"`
	ReblogsCount    int        `json:"reblogs_count"`
	FavouritesCount int        `json:"favourites_count"`
}

// accountRef is the compact account reference embedded in a statusSummary.
type accountRef struct {
	ID          string `json:"id"`
	Acct        string `json:"acct"`
	DisplayName string `json:"display_name"`
}

// postCreated is the compact shape emitted after post create / delete of a
// single status (id, canonical url, visibility, timestamp).
type postCreated struct {
	ID         string `json:"id"`
	URL        string `json:"url"`
	Visibility string `json:"visibility"`
	CreatedAt  string `json:"created_at"`
}

// accountDetail is the provider-neutral account shape emitted by account get
// and whoami. note_text is the HTML bio stripped to plain text.
type accountDetail struct {
	ID             string `json:"id"`
	Acct           string `json:"acct"`
	Username       string `json:"username"`
	DisplayName    string `json:"display_name"`
	NoteText       string `json:"note_text"`
	FollowersCount int    `json:"followers_count"`
	FollowingCount int    `json:"following_count"`
	StatusesCount  int    `json:"statuses_count"`
	URL            string `json:"url"`
}

func summarizeStatus(s rawStatus) statusSummary {
	return statusSummary{
		ID:  s.ID,
		URL: s.URL,
		Account: accountRef{
			ID:          s.Account.ID,
			Acct:        s.Account.Acct,
			DisplayName: s.Account.DisplayName,
		},
		ContentText:     htmlToText(s.Content),
		CreatedAt:       s.CreatedAt,
		Visibility:      s.Visibility,
		RepliesCount:    s.RepliesCount,
		ReblogsCount:    s.ReblogsCount,
		FavouritesCount: s.FavouritesCount,
	}
}

func detailFromAccount(a rawAccount) accountDetail {
	return accountDetail{
		ID:             a.ID,
		Acct:           a.Acct,
		Username:       a.Username,
		DisplayName:    a.DisplayName,
		NoteText:       htmlToText(a.Note),
		FollowersCount: a.FollowersCount,
		FollowingCount: a.FollowingCount,
		StatusesCount:  a.StatusesCount,
		URL:            a.URL,
	}
}

// createdFromStatus builds the compact created/deleted-post shape.
func createdFromStatus(s rawStatus) postCreated {
	return postCreated{ID: s.ID, URL: s.URL, Visibility: s.Visibility, CreatedAt: s.CreatedAt}
}

// decodeStatus / decodeAccount / decodeStatuses decode Mastodon response bodies
// into the raw structs, returning a usage-free apiError on malformed JSON.
func decodeStatus(body []byte) (rawStatus, error) {
	var s rawStatus
	if err := json.Unmarshal(body, &s); err != nil {
		return rawStatus{}, &apiError{msg: "mastodon: decode status response", err: err}
	}
	return s, nil
}

func decodeAccount(body []byte) (rawAccount, error) {
	var a rawAccount
	if err := json.Unmarshal(body, &a); err != nil {
		return rawAccount{}, &apiError{msg: "mastodon: decode account response", err: err}
	}
	return a, nil
}

func decodeStatuses(body []byte) ([]rawStatus, error) {
	var list []rawStatus
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, &apiError{msg: "mastodon: decode status list response", err: err}
	}
	return list, nil
}

var (
	blockCloseRe = regexp.MustCompile(`(?i)</p>|<br\s*/?>`)
	tagRe        = regexp.MustCompile(`(?s)<[^>]*>`)
	multiNLRe    = regexp.MustCompile(`\n{3,}`)
)

// htmlToText converts a Mastodon HTML status/bio body to readable plain text:
// paragraph and line-break boundaries become newlines, all remaining tags are
// dropped, and HTML entities are unescaped. Mastodon serves post content as
// sanitized HTML, so an agent reading content_text never has to parse markup.
func htmlToText(in string) string {
	if in == "" {
		return ""
	}
	out := blockCloseRe.ReplaceAllString(in, "\n")
	out = tagRe.ReplaceAllString(out, "")
	out = html.UnescapeString(out)
	out = multiNLRe.ReplaceAllString(out, "\n\n")
	return strings.TrimSpace(out)
}
