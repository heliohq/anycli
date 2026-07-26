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
		Use:   "request",
		Short: "Submit a raw requestJobDescription (credentials injected automatically)",
		Long: "`--input` is the whole job document as a JSON object and must NOT carry a\n" +
			"`credentials` key — the connected pair is injected, and an input that\n" +
			"includes one is refused locally before any request goes out. This is the\n" +
			"only route to report exports, expense and report writes, and\n" +
			"reconciliation jobs, none of which have typed commands.\n" +
			"\n" +
			"Exporting is TWO calls: a `file` job returns the name of a generated file,\n" +
			"then a second `download` job with that `fileName` returns its contents.\n" +
			"The first call's reply is that bare name, not the data.\n" +
			"\n" +
			"Expensify's templated report exporter takes its Freemarker template as a\n" +
			"separate form field alongside the job, and only the job document is sent\n" +
			"here — so a templated export cannot be driven from this command. Read the\n" +
			"policy or report data and format it locally instead.\n" +
			"\n" +
			"Write jobs run immediately against the real workspace and there is no\n" +
			"dry-run mode, so a create or update job is applied as soon as it is\n" +
			"accepted.",
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
