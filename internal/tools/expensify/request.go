package expensify

import (
	"encoding/json"

	"github.com/spf13/cobra"
)

// newRequestCmd is the raw escape hatch: the caller supplies a complete
// requestJobDescription body (with its own top-level "type" and "inputSettings")
// but WITHOUT credentials, which the service injects. It covers report export
// ("file"→"download"), "create", "update", and "reconciliation" jobs without
// bespoke flags. It is marked side-effecting because those jobs can mutate.
func (s *Service) newRequestCmd(creds credentials) *cobra.Command {
	var input string
	cmd := &cobra.Command{
		Use:         "request",
		Short:       "Submit a raw requestJobDescription (credentials injected automatically)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"}, // create/update/file jobs can mutate
		RunE: func(cmd *cobra.Command, _ []string) error {
			var job map[string]any
			if err := json.Unmarshal([]byte(input), &job); err != nil {
				return &usageError{msg: "--input must be a JSON object (the requestJobDescription body, without credentials): " + err.Error()}
			}
			if _, ok := job["credentials"]; ok {
				return &usageError{msg: "omit credentials from --input; they are injected automatically from the connection"}
			}
			body, err := s.call(cmd.Context(), creds, job)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&input, "input", "", "requestJobDescription JSON body without credentials (required)")
	_ = cmd.MarkFlagRequired("input")
	return cmd
}
