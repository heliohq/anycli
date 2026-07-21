package phantombuster

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newAgentListCmd lists all Phantoms (agents) in the workspace.
// GET /agents/fetch-all (raw array).
func (s *Service) newAgentListCmd(key string) *cobra.Command {
	var inputTypes, outputTypes, ids string
	var withArgument bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all Phantoms in the workspace (GET /agents/fetch-all)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if inputTypes != "" {
				q.Set("inputTypes", inputTypes)
			}
			if outputTypes != "" {
				q.Set("outputTypes", outputTypes)
			}
			if ids != "" {
				q.Set("agentIds", ids)
			}
			if withArgument {
				q.Set("withArgument", "true")
			}
			raw, err := s.call(cmd.Context(), key, http.MethodGet, "/agents/fetch-all", q, nil)
			if err != nil {
				return err
			}
			return s.emitItems(raw)
		},
	}
	cmd.Flags().StringVar(&inputTypes, "input-types", "", "filter by comma-separated input types")
	cmd.Flags().StringVar(&outputTypes, "output-types", "", "filter by comma-separated output types")
	cmd.Flags().StringVar(&ids, "ids", "", "filter to comma-separated agent ids")
	cmd.Flags().BoolVar(&withArgument, "with-argument", false, "include each agent's saved argument")
	return cmd
}

// newAgentGetCmd fetches one Phantom by id.
// GET /agents/fetch?id= (raw object; carries s3Folder used to build result URLs).
func (s *Service) newAgentGetCmd(key string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get one Phantom by id (GET /agents/fetch)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("id", id)
			raw, err := s.call(cmd.Context(), key, http.MethodGet, "/agents/fetch", q, nil)
			if err != nil {
				return err
			}
			return s.emitObject(raw, nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "agent id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

// newAgentLaunchCmd queues a run of a Phantom.
// POST /agents/launch {id, argument?, saveArgument?} → {containerId}.
func (s *Service) newAgentLaunchCmd(key string) *cobra.Command {
	var id, argument string
	var saveArgument bool
	cmd := &cobra.Command{
		Use:   "launch",
		Short: "Queue a run of a Phantom (POST /agents/launch)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{"id": id}
			if argument != "" {
				v, err := decodeJSONFlag("argument", argument)
				if err != nil {
					return err
				}
				body["argument"] = v
			}
			if saveArgument {
				body["saveArgument"] = true
			}
			raw, err := s.call(cmd.Context(), key, http.MethodPost, "/agents/launch", nil, body)
			if err != nil {
				return err
			}
			return s.emitObject(raw, map[string]any{"agent_id": id})
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "agent id to launch")
	cmd.Flags().StringVar(&argument, "argument", "", "launch argument override as a JSON value")
	cmd.Flags().BoolVar(&saveArgument, "save-argument", false, "persist --argument as the agent's default")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

// newAgentOutputCmd fetches the most-recent container's incremental output.
// GET /agents/fetch-output?id=&fromOutputPos= → {output, outputPos, status, isAgentRunning, ...}.
func (s *Service) newAgentOutputCmd(key string) *cobra.Command {
	var id string
	var fromPos int
	cmd := &cobra.Command{
		Use:   "output",
		Short: "Poll a Phantom's most-recent run output (GET /agents/fetch-output)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("id", id)
			if cmd.Flags().Changed("from-pos") {
				q.Set("fromOutputPos", itoa(fromPos))
			}
			raw, err := s.call(cmd.Context(), key, http.MethodGet, "/agents/fetch-output", q, nil)
			if err != nil {
				return err
			}
			return s.emitObject(raw, nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "agent id")
	cmd.Flags().IntVar(&fromPos, "from-pos", 0, "resume output from this position (echo data.output_pos)")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

// newAgentAbortCmd aborts a Phantom's running container(s).
// POST /agents/abort {id}.
func (s *Service) newAgentAbortCmd(key string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "abort",
		Short: "Abort a Phantom's running container(s) (POST /agents/abort)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			raw, err := s.call(cmd.Context(), key, http.MethodPost, "/agents/abort", nil, map[string]any{"id": id})
			if err != nil {
				return err
			}
			return s.emitObject(raw, map[string]any{"agent_id": id, "aborted": true})
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "agent id whose running container(s) to abort")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}
