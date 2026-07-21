package hubspot

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// parseProps turns repeatable --prop key=value pairs into a HubSpot properties
// map. A value may itself contain '=' (only the first splits key from value); a
// pair with no '=' or an empty key is a usage error. An empty slice yields nil.
func parseProps(pairs []string) (map[string]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(pairs))
	for _, raw := range pairs {
		key, value, ok := strings.Cut(raw, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, &usageError{msg: fmt.Sprintf("--prop must be key=value, got %q", raw)}
		}
		out[key] = value
	}
	return out, nil
}

// applyPropertiesQuery adds a comma-joined --properties projection to a query
// value set. HubSpot returns only default properties otherwise, so an agent
// that needs specific fields must name them. Repeated as separate params so
// HubSpot reads each independently.
func applyPropertiesQuery(q url.Values, properties []string) {
	for _, p := range properties {
		for _, name := range strings.Split(p, ",") {
			if name = strings.TrimSpace(name); name != "" {
				q.Add("properties", name)
			}
		}
	}
}

// searchFilter is one HubSpot search predicate.
type searchFilter struct {
	PropertyName string `json:"propertyName"`
	Operator     string `json:"operator"`
	Value        string `json:"value,omitempty"`
}

// parseFilter parses a repeatable --filter "property:operator:value" flag into
// HubSpot filters. operator is upper-cased (EQ, GT, CONTAINS_TOKEN, …). The
// value may contain ':' (only the first two ':' split the triple). HAS_PROPERTY
// / NOT_HAS_PROPERTY take no value ("property:operator" form is accepted).
func parseFilter(raw string) (searchFilter, error) {
	property, rest, ok := strings.Cut(raw, ":")
	if !ok || strings.TrimSpace(property) == "" {
		return searchFilter{}, &usageError{msg: fmt.Sprintf("--filter must be property:operator[:value], got %q", raw)}
	}
	operator, value, hasValue := strings.Cut(rest, ":")
	if strings.TrimSpace(operator) == "" {
		return searchFilter{}, &usageError{msg: fmt.Sprintf("--filter must be property:operator[:value], got %q", raw)}
	}
	f := searchFilter{PropertyName: property, Operator: strings.ToUpper(operator)}
	if hasValue {
		f.Value = value
	}
	return f, nil
}

// searchSort is one HubSpot search sort clause.
type searchSort struct {
	PropertyName string `json:"propertyName"`
	Direction    string `json:"direction"`
}

// parseSort parses "prop" or "prop:desc" into a sort clause (default ASCENDING).
func parseSort(raw string) (searchSort, error) {
	property, dir, ok := strings.Cut(raw, ":")
	if strings.TrimSpace(property) == "" {
		return searchSort{}, &usageError{msg: fmt.Sprintf("--sort must be prop[:asc|desc], got %q", raw)}
	}
	direction := "ASCENDING"
	if ok {
		switch strings.ToLower(dir) {
		case "asc", "ascending":
			direction = "ASCENDING"
		case "desc", "descending":
			direction = "DESCENDING"
		default:
			return searchSort{}, &usageError{msg: fmt.Sprintf("--sort direction must be asc or desc, got %q", dir)}
		}
	}
	return searchSort{PropertyName: property, Direction: direction}, nil
}

// engagementAssoc holds the four id-flag values (repeatable) that map an
// engagement (note/task) to CRM records at creation time.
type engagementAssoc struct {
	contacts  []string
	companies []string
	deals     []string
	tickets   []string
}

// registerAssocFlags wires --contact/--company/--deal/--ticket (repeatable) for
// engagement create commands.
func registerAssocFlags(cmd *cobra.Command, a *engagementAssoc) {
	cmd.Flags().StringArrayVar(&a.contacts, "contact", nil, "associate with a contact id (repeatable)")
	cmd.Flags().StringArrayVar(&a.companies, "company", nil, "associate with a company id (repeatable)")
	cmd.Flags().StringArrayVar(&a.deals, "deal", nil, "associate with a deal id (repeatable)")
	cmd.Flags().StringArrayVar(&a.tickets, "ticket", nil, "associate with a ticket id (repeatable)")
}

// assocEntry is one inline associations[] element on an engagement create.
type assocEntry struct {
	To    assocTarget `json:"to"`
	Types []assocType `json:"types"`
}

type assocTarget struct {
	ID string `json:"id"`
}

type assocType struct {
	AssociationCategory string `json:"associationCategory"`
	AssociationTypeID   int    `json:"associationTypeId"`
}

// hubspotDefined is HubSpot's category for its built-in association type ids.
const hubspotDefined = "HUBSPOT_DEFINED"

// engagementAssocTypeID is the HUBSPOT_DEFINED default association type id per
// (engagement, target) pair — the canonical values from HubSpot's associations
// v4 reference. note→{contact 202, company 190, deal 214, ticket 228};
// task→{contact 204, company 192, deal 216, ticket 230}.
var engagementAssocTypeID = map[string]map[string]int{
	"notes": {"contact": 202, "company": 190, "deal": 214, "ticket": 228},
	"tasks": {"contact": 204, "company": 192, "deal": 216, "ticket": 230},
}

// buildEngagementAssociations turns the four id-flag slices into inline
// associations[] entries for the given engagement object type (notes|tasks).
func buildEngagementAssociations(engagement string, a engagementAssoc) []assocEntry {
	table := engagementAssocTypeID[engagement]
	var entries []assocEntry
	add := func(ids []string, target string) {
		for _, id := range ids {
			entries = append(entries, assocEntry{
				To:    assocTarget{ID: id},
				Types: []assocType{{AssociationCategory: hubspotDefined, AssociationTypeID: table[target]}},
			})
		}
	}
	add(a.contacts, "contact")
	add(a.companies, "company")
	add(a.deals, "deal")
	add(a.tickets, "ticket")
	return entries
}

// resolveTimestamp returns the explicit value when set, otherwise the current
// time as an RFC 3339 (UTC, millisecond) string — HubSpot requires hs_timestamp
// on engagement create and accepts ISO-8601.
func resolveTimestamp(explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}

// applyPaging adds --limit / --after to a query value set when set.
func applyPaging(q url.Values, limit int, after string) {
	if limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	if after != "" {
		q.Set("after", after)
	}
}
