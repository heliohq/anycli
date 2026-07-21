package iterable

import "strings"

// Data-center base URLs. A key minted in one data center returns auth errors
// against the other host; there is no cross-DC routing, so the region is part
// of the credential and picks the host explicitly (no auto-probe).
const (
	baseURLUS = "https://api.iterable.com"
	baseURLEU = "https://api.eu.iterable.com"
)

// regionBaseURLs maps the reviewed region tokens onto their data-center host.
var regionBaseURLs = map[string]string{
	"us": baseURLUS,
	"eu": baseURLEU,
}

// credential is the decoded injected secret: the raw Api-Key value plus the
// data-center base URL its region selected. The optional account alias is a
// Helio-side identity fact only and is not retained here — anycli needs only
// the region (→ host) and the key.
type credential struct {
	apiKey  string
	baseURL string
}

// parseCredential splits the injected ITERABLE_API_KEY secret
// "<region>[:<alias>]:<key>" into 2 or 3 colon segments:
//
//   - segment[0] is the region: "us" → api.iterable.com, "eu" →
//     api.eu.iterable.com. Any other region is a fail-fast usage error (no
//     silent default — a wrong host would leak the key to the wrong DC).
//   - the LAST segment is the raw project key sent as the Api-Key header.
//   - a present MIDDLE segment (3-part form) is the account alias, ignored here.
//
// Iterable keys are alphanumeric and never contain a colon, and the alias is
// colon-free by construction (a colon in the alias pushes the count past 3), so
// the split is unambiguous. A part count outside {2,3}, an unknown/empty
// region, or an empty key is a usageError (exit 2).
func parseCredential(secret string) (credential, error) {
	if strings.TrimSpace(secret) == "" {
		return credential{}, &usageError{msg: EnvAPIKey + " is not set"}
	}
	parts := strings.Split(secret, ":")
	if len(parts) != 2 && len(parts) != 3 {
		return credential{}, &usageError{msg: credentialFormatMsg}
	}
	region := parts[0]
	key := parts[len(parts)-1]
	baseURL, ok := regionBaseURLs[region]
	if !ok {
		return credential{}, &usageError{msg: "unknown Iterable region " + quote(region) + " (want \"us\" or \"eu\"); " + credentialFormatMsg}
	}
	if strings.TrimSpace(key) == "" {
		return credential{}, &usageError{msg: "empty Iterable API key; " + credentialFormatMsg}
	}
	return credential{apiKey: key, baseURL: baseURL}, nil
}

const credentialFormatMsg = "paste the credential as \"<region>:<key>\" or \"<region>:<alias>:<key>\" (region is \"us\" or \"eu\")"

// quote wraps a value in double quotes for error messages without pulling in
// fmt %q escaping of a user-facing token.
func quote(s string) string { return "\"" + s + "\"" }
