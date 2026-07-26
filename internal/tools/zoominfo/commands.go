package zoominfo

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// newBodyCmd builds a POST command whose request body is supplied as JSON by
// the AI (--body inline or --file path). ZoomInfo's Search and Enrich request
// schemas are rich and evolving (Legacy -> New API migration); a JSON
// passthrough keeps the surface valid without a field-by-field rebuild, and
// the AI discovers valid filters/outputFields via `lookup`. The response body
// is emitted verbatim so the assistant reads record ids, data, and — for
// enrich — the credit consumption the response reports.
// The four search/enrich Longs live here, next to the shared builder, because
// it is the builder that fixes the JSON-body contract and the endpoint all four
// describe; only the resource and the credit cost differ between them.
const (
	longContactSearch = "Returns candidate `personId` values with light identity hints — never the\n" +
		"email or direct phone, which is what `contact enrich` exists for. The body\n" +
		"takes ZoomInfo's contact filters (job title, company name or domain,\n" +
		"management level, department, location); `lookup inputFields/contact` is the\n" +
		"authoritative list of them. Spends no credit, so narrow the candidate set\n" +
		"here before spending anything."

	longContactEnrich = "At most 25 records per call; a larger set has to be batched across calls.\n" +
		"The body pairs `matchPersonInput` — an array of `personId`s from\n" +
		"`contact search`, or match keys such as an email plus a company — with an\n" +
		"explicit `outputFields` list. Omitting `outputFields` gets ZoomInfo's\n" +
		"default projection rather than the fields actually wanted.\n" +
		"\n" +
		"One credit per record that has not already been enriched in the last 12\n" +
		"months; a repeat inside that window is free. The response reports what the\n" +
		"call actually consumed, which is the only feedback on cost."

	longCompanySearch = "Returns candidate `companyId` values with firmographic hints. The body takes\n" +
		"company filters (name, website domain, industry, employee count, revenue,\n" +
		"location) — `lookup inputFields/company` is the authoritative list. A domain\n" +
		"is far more selective than a name, which collides across regions and\n" +
		"subsidiaries. Spends no credit."

	longCompanyEnrich = "At most 25 companies per call, keyed by `companyId` from `company search` or\n" +
		"by a website domain, with an explicit `outputFields` list for the\n" +
		"firmographics needed. One credit per record newly enriched, free again for\n" +
		"the next 12 months, and the response reports the spend. Enriching a whole\n" +
		"search page speculatively is how a monthly allotment disappears."
)

func (s *Service) newBodyCmd(st *runState, use, short, long, path string) *cobra.Command {
	var body, file string
	cmd := &cobra.Command{
		Use:           use,
		Short:         short,
		Long:          long,
		Annotations:   readOnly,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := readJSONBody(body, file)
			if err != nil {
				return err
			}
			token, err := s.accessToken(cmd.Context(), st)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, path, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&body, "body", "", "request body as inline JSON")
	cmd.Flags().StringVar(&file, "file", "", "path to a file containing the request body JSON (\"-\" for stdin)")
	return cmd
}

// newLookupCmd exposes ZoomInfo's read-only field-discovery surface:
// GET /lookup/<resource> (for example `lookup inputFields/contact` or
// `lookup outputFields/company`). No credit is consumed.
func (s *Service) newLookupCmd(st *runState) *cobra.Command {
	return &cobra.Command{
		Use:   "lookup <resource>",
		Short: "Discover valid input filters / output fields (no credit)",
		Long: "The resource is a path segment: `inputFields/contact`,\n" +
			"`outputFields/contact`, `inputFields/company`, `outputFields/company`. This\n" +
			"is the only source of truth for the field names a search or enrich body may\n" +
			"carry — ZoomInfo is mid-migration from its Legacy Enterprise API to a new\n" +
			"one and the names differ between them, so a rejected filter or output field\n" +
			"is a signal to run this rather than to retry. Spends no credit.",
		Annotations:   readOnly,
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			resource := strings.Trim(strings.TrimSpace(args[0]), "/")
			if resource == "" {
				return &usageError{msg: "lookup requires a non-empty resource path"}
			}
			token, err := s.accessToken(cmd.Context(), st)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/lookup/"+resource, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

// newUsageCmd reports remaining credits / request limits against the monthly
// allotment so the assistant can check cost before enriching. No credit.
func (s *Service) newUsageCmd(st *runState) *cobra.Command {
	return &cobra.Command{
		Use:   "usage",
		Short: "Report remaining API credits and request limits (no credit)",
		Long: "Reports the remaining credit balance and request limits against the monthly\n" +
			"allotment. Free to call, and worth calling before any sizeable\n" +
			"`contact enrich` / `company enrich` batch: enrich is the only thing in this\n" +
			"tool that spends, at one credit per newly enriched record, and it will not\n" +
			"stop partway to warn about the balance.",
		Annotations:   readOnly,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			token, err := s.accessToken(cmd.Context(), st)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/usage", nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

// readJSONBody resolves the request body from --body or --file (mutually
// exclusive) and validates it is well-formed JSON before any network call, so
// a malformed body is a usage error (exit 2), not a burned API request.
func readJSONBody(body, file string) ([]byte, error) {
	body = strings.TrimSpace(body)
	if body != "" && file != "" {
		return nil, &usageError{msg: "provide either --body or --file, not both"}
	}
	var raw []byte
	switch {
	case body != "":
		raw = []byte(body)
	case file == "-":
		b, err := readAllStdin()
		if err != nil {
			return nil, &usageError{msg: fmt.Sprintf("read body from stdin: %v", err)}
		}
		raw = b
	case file != "":
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, &usageError{msg: fmt.Sprintf("read body file: %v", err)}
		}
		raw = b
	default:
		return nil, &usageError{msg: "a request body is required (--body or --file)"}
	}
	if !json.Valid(raw) {
		return nil, &usageError{msg: "request body is not valid JSON"}
	}
	return raw, nil
}

func readAllStdin() ([]byte, error) {
	info, err := os.Stdin.Stat()
	if err == nil && (info.Mode()&os.ModeCharDevice) != 0 {
		return nil, fmt.Errorf("no data on stdin")
	}
	return io.ReadAll(os.Stdin)
}
