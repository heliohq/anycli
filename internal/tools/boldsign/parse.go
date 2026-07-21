package boldsign

import (
	"encoding/base64"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// parsedParty is a name+email pair parsed from a "Name <email>" spec, shared by
// document signers and template roles.
type parsedParty struct {
	name  string
	email string
}

// parseParty parses a "Name <email>" spec. The name is required (BoldSign
// requires a signer/role name); a bare email with no angle brackets is a usage
// error so the agent gets a clear message rather than a provider 400.
func parseParty(spec string) (parsedParty, error) {
	spec = strings.TrimSpace(spec)
	open := strings.LastIndex(spec, "<")
	close := strings.LastIndex(spec, ">")
	if open < 0 || close < 0 || close < open {
		return parsedParty{}, &usageError{msg: fmt.Sprintf("boldsign: %q must be in the form \"Name <email>\"", spec)}
	}
	name := strings.TrimSpace(spec[:open])
	email := strings.TrimSpace(spec[open+1 : close])
	if name == "" {
		return parsedParty{}, &usageError{msg: fmt.Sprintf("boldsign: %q is missing a name; use \"Name <email>\"", spec)}
	}
	if email == "" {
		return parsedParty{}, &usageError{msg: fmt.Sprintf("boldsign: %q is missing an email; use \"Name <email>\"", spec)}
	}
	return parsedParty{name: name, email: email}, nil
}

// parseRoleSpec parses a "<roleIndex>:Name <email>" spec into a role index and a
// party. roleIndex must be an integer in [1,50] per BoldSign's template roles.
func parseRoleSpec(spec string) (int, parsedParty, error) {
	spec = strings.TrimSpace(spec)
	colon := strings.Index(spec, ":")
	if colon < 0 {
		return 0, parsedParty{}, &usageError{msg: fmt.Sprintf("boldsign: %q must be in the form \"<roleIndex>:Name <email>\"", spec)}
	}
	idxRaw := strings.TrimSpace(spec[:colon])
	index, err := strconv.Atoi(idxRaw)
	if err != nil {
		return 0, parsedParty{}, &usageError{msg: fmt.Sprintf("boldsign: role index %q is not an integer", idxRaw)}
	}
	if index < 1 || index > 50 {
		return 0, parsedParty{}, &usageError{msg: fmt.Sprintf("boldsign: role index %d must be between 1 and 50", index)}
	}
	party, err := parseParty(spec[colon+1:])
	if err != nil {
		return 0, parsedParty{}, err
	}
	return index, party, nil
}

// parseFieldSpec parses an "<id>=<value>" prefill spec into an id and value.
func parseFieldSpec(spec string) (id, value string, err error) {
	eq := strings.Index(spec, "=")
	if eq < 0 {
		return "", "", &usageError{msg: fmt.Sprintf("boldsign: %q must be in the form \"<fieldId>=<value>\"", spec)}
	}
	id = strings.TrimSpace(spec[:eq])
	value = spec[eq+1:]
	if id == "" {
		return "", "", &usageError{msg: fmt.Sprintf("boldsign: %q is missing a field id", spec)}
	}
	return id, value, nil
}

// fileEntry is one entry of the send-document Files array (JSON object form):
// a data-URI base64 payload plus the original file name.
type fileEntry struct {
	Base64   string `json:"base64"`
	FileName string `json:"fileName"`
}

// readFileEntry reads a local file and encodes it as the data-URI object form
// BoldSign's JSON Files array expects. The MIME type is derived from the
// extension, defaulting to application/pdf (BoldSign's preferred format).
func readFileEntry(path string) (fileEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return fileEntry{}, &usageError{msg: fmt.Sprintf("boldsign: read %s: %v", path, err)}
	}
	name := filepath.Base(path)
	dataURI := "data:" + fileMIME(path) + ";base64," + base64.StdEncoding.EncodeToString(data)
	return fileEntry{Base64: dataURI, FileName: name}, nil
}

// fileMIME resolves a request MIME type from a file extension, defaulting to
// application/pdf when the extension is unknown.
func fileMIME(path string) string {
	if mt := mime.TypeByExtension(strings.ToLower(filepath.Ext(path))); mt != "" {
		// Strip any "; charset=..." suffix — the data URI needs the bare type.
		if semi := strings.Index(mt, ";"); semi >= 0 {
			mt = mt[:semi]
		}
		return strings.TrimSpace(mt)
	}
	return "application/pdf"
}
