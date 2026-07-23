package moz

import (
	"encoding/json"

	"github.com/spf13/cobra"
)

// newCallCmd is the generic JSON-RPC escape hatch: it invokes any Moz method
// by name with a raw --data JSON body, wrapped in the same 2.0 envelope and
// x-moz-token auth as the typed subcommands. It exists so an agent is never
// blocked on a method this tree does not wrap — and so any method whose typed
// params shape differs from Moz's evolving schema can still be reached with the
// documented body verbatim.
func (s *Service) newCallCmd(token string) *cobra.Command {
	var method, dataJSON string
	cmd := &cobra.Command{
		Use:         "call",
		Short:       "Invoke any Moz JSON-RPC method with a raw --data JSON body",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if method == "" {
				return &usageError{msg: "moz: --method is required (e.g. data.site.metrics.fetch)"}
			}
			var data any = map[string]any{}
			if dataJSON != "" {
				if err := json.Unmarshal([]byte(dataJSON), &data); err != nil {
					return &usageError{msg: "moz: --data is not valid JSON: " + err.Error()}
				}
			}
			result, err := s.call(cmd.Context(), token, method, data)
			if err != nil {
				return err
			}
			return s.emit(result)
		},
	}
	cmd.Flags().StringVar(&method, "method", "", "JSON-RPC method name, e.g. data.site.metrics.fetch")
	cmd.Flags().StringVar(&dataJSON, "data", "", "params.data as a raw JSON object (default: {})")
	return cmd
}
