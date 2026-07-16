package contacts

import (
	"fmt"
	"io"
	"strings"
)

// person is the subset of the People API Person resource the human-readable
// views and `resolve` synthesis need. --json paths pass the raw provider body
// through untouched; this decode only feeds the summary rendering.
type person struct {
	ResourceName string `json:"resourceName"`
	Names        []struct {
		DisplayName string `json:"displayName"`
		GivenName   string `json:"givenName"`
		FamilyName  string `json:"familyName"`
		Metadata    struct {
			Primary bool `json:"primary"`
		} `json:"metadata"`
	} `json:"names"`
	EmailAddresses []struct {
		Value    string `json:"value"`
		Metadata struct {
			Primary bool `json:"primary"`
		} `json:"metadata"`
	} `json:"emailAddresses"`
	PhoneNumbers []struct {
		Value string `json:"value"`
	} `json:"phoneNumbers"`
	Organizations []struct {
		Name  string `json:"name"`
		Title string `json:"title"`
	} `json:"organizations"`
}

// displayName returns the best human label: the primary name's displayName,
// else the first name, else a givenName/familyName join, else "(no name)".
func (p *person) displayName() string {
	if len(p.Names) == 0 {
		return "(no name)"
	}
	pick := p.Names[0]
	for _, n := range p.Names {
		if n.Metadata.Primary {
			pick = n
			break
		}
	}
	if pick.DisplayName != "" {
		return pick.DisplayName
	}
	joined := strings.TrimSpace(pick.GivenName + " " + pick.FamilyName)
	if joined != "" {
		return joined
	}
	return "(no name)"
}

// emails returns every email value, primary first.
func (p *person) emails() []string {
	out := make([]string, 0, len(p.EmailAddresses))
	for _, e := range p.EmailAddresses {
		if e.Metadata.Primary && e.Value != "" {
			out = append([]string{e.Value}, out...)
			continue
		}
		if e.Value != "" {
			out = append(out, e.Value)
		}
	}
	return out
}

// primaryEmail returns the first (primary-preferred) email, or "".
func (p *person) primaryEmail() string {
	if e := p.emails(); len(e) > 0 {
		return e[0]
	}
	return ""
}

// phones returns every phone value.
func (p *person) phones() []string {
	out := make([]string, 0, len(p.PhoneNumbers))
	for _, n := range p.PhoneNumbers {
		if n.Value != "" {
			out = append(out, n.Value)
		}
	}
	return out
}

// organization returns the first organization as "Name — Title" (either part
// may be blank), or "".
func (p *person) organization() string {
	if len(p.Organizations) == 0 {
		return ""
	}
	o := p.Organizations[0]
	switch {
	case o.Name != "" && o.Title != "":
		return o.Name + " — " + o.Title
	case o.Name != "":
		return o.Name
	default:
		return o.Title
	}
}

// dedupKey is the merge key for `resolve`: the primary email if present, else
// the resourceName. Contacts with no email never collapse into each other.
func (p *person) dedupKey() string {
	if e := p.primaryEmail(); e != "" {
		return "email:" + strings.ToLower(e)
	}
	return "rn:" + p.ResourceName
}

// writeLine prints a one-line summary: name, primary email, primary phone.
func writeLine(w io.Writer, p *person) {
	parts := []string{p.displayName()}
	if e := p.primaryEmail(); e != "" {
		parts = append(parts, e)
	}
	if ph := p.phones(); len(ph) > 0 {
		parts = append(parts, ph[0])
	}
	fmt.Fprintf(w, "%s\t%s\n", p.ResourceName, strings.Join(parts, "  "))
}
