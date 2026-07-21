package lusha

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// contactRevealFields are the only values Lusha's contact reveal selector
// accepts (V3 contacts/enrich + contacts/search-and-enrich reveal enum).
var contactRevealFields = map[string]bool{"emails": true, "phones": true}

func (s *Service) newContactCmd(key string) *cobra.Command {
	cmd := newGroupCmd("contact", "Contacts (enrich by identifier, prospect by filter, reveal by id)")
	cmd.AddCommand(
		s.newContactEnrichCmd(key),
		s.newContactSearchCmd(key),
		s.newContactRevealCmd(key),
	)
	return cmd
}

// newContactEnrichCmd is the one-shot known-identifier path: turn an
// email / LinkedIn URL / name+company into a revealed contact via
// POST /contacts/search-and-enrich. Charges twice (api_search + reveal).
func (s *Service) newContactEnrichCmd(key string) *cobra.Command {
	var email, linkedinURL, firstName, lastName, companyName, companyDomain, reveal string
	cmd := &cobra.Command{
		Use:   "enrich",
		Short: "Enrich a known contact by identifier (POST /contacts/search-and-enrich)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			item := map[string]any{}
			addIfSet(item, "email", email)
			addIfSet(item, "linkedinUrl", linkedinURL)
			addIfSet(item, "firstName", firstName)
			addIfSet(item, "lastName", lastName)
			addIfSet(item, "companyName", companyName)
			addIfSet(item, "companyDomain", companyDomain)
			if len(item) == 0 {
				return &usageError{msg: "provide at least one identifier: --email, --linkedin-url, or --first-name/--last-name with --company-name/--company-domain"}
			}
			body := map[string]any{"contacts": []any{item}}
			revealed, err := revealValues(reveal, contactRevealFields)
			if err != nil {
				return err
			}
			if revealed != nil {
				body["reveal"] = revealed
			}
			resp, err := s.call(cmd.Context(), key, http.MethodPost, "/contacts/search-and-enrich", body)
			if err != nil {
				return err
			}
			return s.emitRevealEnvelope(resp)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "contact email address")
	cmd.Flags().StringVar(&linkedinURL, "linkedin-url", "", "contact LinkedIn profile URL")
	cmd.Flags().StringVar(&firstName, "first-name", "", "contact first name (with --last-name + company)")
	cmd.Flags().StringVar(&lastName, "last-name", "", "contact last name (with --first-name + company)")
	cmd.Flags().StringVar(&companyName, "company-name", "", "company name (with a name pair)")
	cmd.Flags().StringVar(&companyDomain, "company-domain", "", "company domain (with a name pair)")
	cmd.Flags().StringVar(&reveal, "reveal", "", "comma-separated fields to reveal: emails,phones (omit = both)")
	return cmd
}

// newContactSearchCmd is ICP prospecting: filter by the V3 contact-prospecting
// filter DSL → net-new contact ids + a request id (name-only preview, no
// email/phone). The filter body is large and nested, so it is passed as raw
// JSON (--filters) rather than a flag per field; the AI-facing sub-doc
// documents the shape. Charged api_search per result.
func (s *Service) newContactSearchCmd(key string) *cobra.Command {
	var filtersJSON string
	var page, size int
	var includePartial bool
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Prospect net-new contacts by filter (POST /contacts/prospecting)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			filters, err := decodeFiltersFlag(filtersJSON)
			if err != nil {
				return err
			}
			body := map[string]any{
				"pagination": map[string]any{"page": page, "size": size},
				"filters":    filters,
			}
			if cmd.Flags().Changed("include-partial") {
				body["options"] = map[string]any{"includePartialProfiles": includePartial}
			}
			resp, err := s.call(cmd.Context(), key, http.MethodPost, "/contacts/prospecting", body)
			if err != nil {
				return err
			}
			return s.emitSearchEnvelope(resp)
		},
	}
	registerProspectingFlags(cmd, &filtersJSON, &page, &size, &includePartial)
	return cmd
}

// newContactRevealCmd is the reveal step for prospecting: takes up to 100 Lusha
// contact ids (from a search result) + a reveal selector → full revealed
// records via POST /contacts/enrich. Charged per revealed datapoint only (no
// api_search) — the credit-efficient path.
func (s *Service) newContactRevealCmd(key string) *cobra.Command {
	var ids []string
	var reveal string
	cmd := &cobra.Command{
		Use:   "reveal",
		Short: "Reveal contacts by Lusha id (POST /contacts/enrich)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateIDs(ids); err != nil {
				return err
			}
			body := map[string]any{"ids": ids}
			revealed, err := revealValues(reveal, contactRevealFields)
			if err != nil {
				return err
			}
			if revealed != nil {
				body["reveal"] = revealed
			}
			resp, err := s.call(cmd.Context(), key, http.MethodPost, "/contacts/enrich", body)
			if err != nil {
				return err
			}
			return s.emitRevealEnvelope(resp)
		},
	}
	cmd.Flags().StringArrayVar(&ids, "id", nil, "Lusha contact id to reveal (repeatable, 1-100)")
	cmd.Flags().StringVar(&reveal, "reveal", "", "comma-separated fields to reveal: emails,phones (omit = both)")
	return cmd
}

// addIfSet writes a non-empty value into the identifier map.
func addIfSet(m map[string]any, key, value string) {
	if value != "" {
		m[key] = value
	}
}

// revealValues validates a comma-separated --reveal flag against the allowed
// enum for the verb. An empty flag returns nil (omit the key = provider
// default). An unknown value is a usage error.
func revealValues(raw string, allowed map[string]bool) ([]string, error) {
	values := splitCSV(raw)
	if len(values) == 0 {
		return nil, nil
	}
	for _, v := range values {
		if !allowed[v] {
			return nil, &usageError{msg: fmt.Sprintf("invalid --reveal value %q (allowed: %s)", v, allowedList(allowed))}
		}
	}
	return values, nil
}

// allowedList renders an allowed-value set as a stable, sorted, comma list for
// error messages.
func allowedList(allowed map[string]bool) string {
	keys := make([]string, 0, len(allowed))
	for k := range allowed {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

// validateIDs enforces the 1-100 batch bound Lusha's enrich endpoints require.
func validateIDs(ids []string) error {
	if len(ids) == 0 {
		return &usageError{msg: "provide at least one --id"}
	}
	if len(ids) > 100 {
		return &usageError{msg: fmt.Sprintf("too many ids: %d (max 100 per call)", len(ids))}
	}
	return nil
}

// registerProspectingFlags wires the shared prospecting flags: the raw --filters
// JSON body plus pagination + include-partial.
func registerProspectingFlags(cmd *cobra.Command, filtersJSON *string, page, size *int, includePartial *bool) {
	cmd.Flags().StringVar(filtersJSON, "filters", "", "prospecting filter object as JSON (see the Lusha tool doc for the shape)")
	cmd.Flags().IntVar(page, "page", 0, "page number (0-based, 0-1000)")
	cmd.Flags().IntVar(size, "size", 25, "results per page (10-100)")
	cmd.Flags().BoolVar(includePartial, "include-partial", false, "include partial profiles in results")
	_ = cmd.MarkFlagRequired("filters")
}

// decodeFiltersFlag validates the raw --filters JSON and returns the decoded
// object for the prospecting request body.
func decodeFiltersFlag(raw string) (any, error) {
	if raw == "" {
		return nil, &usageError{msg: "--filters is required"}
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("--filters is not valid JSON: %v", err)}
	}
	return v, nil
}
