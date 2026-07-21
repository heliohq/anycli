package forms

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/spf13/cobra"
)

// newBatchUpdateCmd builds `forms batch-update`. The batchUpdate Request[] JSON
// is passed through verbatim — createItem / updateItem / deleteItem / moveItem /
// updateFormInfo / updateSettings all live here. No second question-type DSL is
// invented; the AI already knows the Request union.
func (s *Service) newBatchUpdateCmd(token string) *cobra.Command {
	var requests, requestsFile string
	cmd := &cobra.Command{
		Use:   "batch-update <form-id> (--requests <json> | --requests-file <path>)",
		Short: "Apply a batchUpdate Request[] to a form (create/update/delete/move items, form info, settings)",
		Args:  cobra.ExactArgs(1),
		// POST /forms/{id}:batchUpdate — mutating provider call (design 318).
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			formID, err := extractFormID(args[0])
			if err != nil {
				return err
			}
			raw, err := readRequests(requests, requestsFile)
			if err != nil {
				return err
			}
			// Accept either a bare Request[] array or a full batchUpdate body
			// ({"requests":[...]}); normalize to the batchUpdate body.
			payload, err := buildBatchPayload(raw)
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/forms/"+url.PathEscape(formID)+":batchUpdate", nil, payload)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			fmt.Fprintf(s.stdout(), "batch-update applied to form %s\n", formID)
			return nil
		},
	}
	cmd.Flags().StringVar(&requests, "requests", "", "batchUpdate Request[] JSON (array, or a full {\"requests\":[...]} body)")
	cmd.Flags().StringVar(&requestsFile, "requests-file", "", "path to a file holding the same JSON as --requests")
	cmd.MarkFlagsMutuallyExclusive("requests", "requests-file")
	return cmd
}

// readRequests returns the requests JSON from the inline flag or a file. Exactly
// one source must be provided.
func readRequests(inline, file string) ([]byte, error) {
	switch {
	case inline != "" && file != "":
		return nil, fmt.Errorf("forms: pass only one of --requests or --requests-file")
	case inline != "":
		return []byte(inline), nil
	case file != "":
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("forms: read --requests-file: %w", err)
		}
		return b, nil
	default:
		return nil, fmt.Errorf("forms: provide --requests <json> or --requests-file <path>")
	}
}

// buildBatchPayload normalizes the caller's JSON into a batchUpdate request
// body. It accepts a bare Request[] array or a full {"requests":[...]} object
// so the AI can pass either shape it already knows.
func buildBatchPayload(raw []byte) (map[string]json.RawMessage, error) {
	trimmed := trimLeadingSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("forms: empty batch-update requests")
	}
	if trimmed[0] == '[' {
		var arr json.RawMessage
		if err := json.Unmarshal(raw, &arr); err != nil {
			return nil, fmt.Errorf("forms: --requests is not valid JSON: %w", err)
		}
		return map[string]json.RawMessage{"requests": arr}, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("forms: --requests is not valid JSON: %w", err)
	}
	if _, ok := obj["requests"]; !ok {
		return nil, fmt.Errorf("forms: --requests object must contain a \"requests\" array")
	}
	return obj, nil
}

// trimLeadingSpace returns raw with leading ASCII whitespace removed.
func trimLeadingSpace(raw []byte) []byte {
	i := 0
	for i < len(raw) {
		switch raw[i] {
		case ' ', '\t', '\r', '\n':
			i++
		default:
			return raw[i:]
		}
	}
	return raw[i:]
}
