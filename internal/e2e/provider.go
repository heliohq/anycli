// Package e2e is the e2e test support package: a CredentialResolver backed
// by Helio's integration token gateway (design 008), an env-var override
// for local pre-integration testing, and a stdout-capturing tool runner.
//
// The package carries no build tag so it is compiled and unit-tested by the
// normal `go test ./...` run; only the per-service e2e_test.go files (which
// hit real provider APIs) are behind the `e2e` build tag.
package e2e

// toolToProvider maps anycli tool names to Helio provider catalog keys
// where the two differ. Identity holds for every other tool. This is a
// copy of helio-cli/internal/toolcred.toolToProvider (not importable:
// internal package of another module), updated for anycli's current tool
// ids (bill-com→billcom, customer-io→customerio were folded by c269a6e).
// Keep in sync with that table; the source of truth for the provider keys
// is Helio's provider catalog.
var toolToProvider = map[string]string{
	"adobe-sign":         "adobe_sign",
	"billcom":            "bill_com",
	"customerio":         "customer_io",
	"dropbox-sign":       "dropbox_sign",
	"facebook-pages":     "facebook_pages",
	"google-ads":         "google_ads",
	"google-analytics":   "google_analytics",
	"help-scout":         "help_scout",
	"lemon-squeezy":      "lemon_squeezy",
	"meta-ads":           "meta_ads",
	"microsoft-calendar": "microsoft_calendar",
	"microsoft-onedrive": "microsoft_onedrive",
	"microsoft-outlook":  "microsoft_outlook",
	"search-console":     "google_search_console",
	"sprout-social":      "sprout_social",
	"zoho-books":         "zoho_books",
	"zoho-crm":           "zoho_crm",
	// Google short-name family (design 303 on the Helio side).
	"calendar": "google_calendar",
	"contacts": "google_contacts",
	"docs":     "google_docs",
	"drive":    "google_drive",
	"forms":    "google_forms",
	"meet":     "google_meet",
	"sheets":   "google_sheets",
	"slides":   "google_slides",
	"tasks":    "google_tasks",
}

// ProviderFor returns the Helio provider catalog key for an anycli tool.
func ProviderFor(tool string) string {
	if p, ok := toolToProvider[tool]; ok {
		return p
	}
	return tool
}
