package posthog

import (
	"encoding/json"

	"github.com/spf13/cobra"
)

// newQueryCmd groups the ad-hoc analytics query surface.
func (s *Service) newQueryCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "query", Short: "Ad-hoc analytics queries (HogQL or raw query nodes)"}
	cmd.AddCommand(s.newQueryRunCmd(token))
	return cmd
}

// newQueryRunCmd runs one query against POST /api/projects/<id>/query/. Exactly
// one of --hogql (a SQL string wrapped as a HogQLQuery node) or --query-json (a
// raw query node read from a file or stdin, for advanced kinds like
// TrendsQuery / FunnelsQuery) must be supplied. Both wrap under {"query": …}.
func (s *Service) newQueryRunCmd(token string) *cobra.Command {
	var project, hogql, queryJSON string
	cmd := &cobra.Command{
		Use:         "run",
		Short:       "Run a HogQL or raw-node query (POST /api/projects/<id>/query/)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireProject(project); err != nil {
				return err
			}
			node, err := queryNode(cmd, hogql, queryJSON)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, "POST", projectPath(project, "/query/"), nil, map[string]any{"query": node})
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project id (required)")
	cmd.Flags().StringVar(&hogql, "hogql", "", "HogQL query string (wrapped as a HogQLQuery node)")
	cmd.Flags().StringVar(&queryJSON, "query-json", "", "raw query node as a JSON file path, or - for stdin")
	return cmd
}

// queryNode resolves the query node from exactly one of --hogql / --query-json.
func queryNode(cmd *cobra.Command, hogql, queryJSON string) (any, error) {
	switch {
	case hogql != "" && queryJSON != "":
		return nil, &usageError{msg: "pass only one of --hogql or --query-json"}
	case hogql != "":
		return map[string]any{"kind": "HogQLQuery", "query": hogql}, nil
	case queryJSON != "":
		raw, err := readFileOrStdin(cmd, queryJSON)
		if err != nil {
			return nil, &usageError{msg: "read --query-json: " + err.Error()}
		}
		var node any
		if err := json.Unmarshal(raw, &node); err != nil {
			return nil, &usageError{msg: "--query-json is not valid JSON: " + err.Error()}
		}
		return node, nil
	default:
		return nil, &usageError{msg: "one of --hogql or --query-json is required"}
	}
}
