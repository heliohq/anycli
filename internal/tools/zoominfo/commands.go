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
func (s *Service) newBodyCmd(st *runState, use, short, path string) *cobra.Command {
	var body, file string
	cmd := &cobra.Command{
		Use:           use,
		Short:         short,
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
		Use:           "lookup <resource>",
		Short:         "Discover valid input filters / output fields (no credit)",
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
		Use:           "usage",
		Short:         "Report remaining API credits and request limits (no credit)",
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
