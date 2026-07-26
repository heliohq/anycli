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
		Long: "Returns every Phantom in the workspace in one unpaginated response.\n" +
			"--input-types and --output-types take comma-separated type names and are\n" +
			"the practical way to narrow a large workspace; --ids selects specific\n" +
			"agents. --with-argument adds each Phantom's saved argument JSON, which is\n" +
			"how to learn what a Phantom expects before overriding it — it is off by\n" +
			"default because those arguments are bulky and often hold session cookies.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "--id is the agent id. The response carries the Phantom's script reference,\n" +
			"its currently saved argument and its `s3Folder`, which is half of the\n" +
			"public URL a finished run's result file sits at — the other half comes\n" +
			"from `org get`. Reading a Phantom never runs it; that is `agent launch`.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "Queues one run and returns a `containerId` immediately; no data comes back\n" +
			"from this call. --argument is a JSON value that overrides the Phantom's\n" +
			"saved argument for THIS RUN only — unless --save-argument is also passed,\n" +
			"which rewrites the Phantom's stored default and therefore changes every\n" +
			"future launch, including ones PhantomBuster's own scheduler triggers.\n" +
			"Launching without --argument re-uses whatever is saved, session cookies\n" +
			"included. Confirm remaining budget with `org resources` first: a run that\n" +
			"exhausts execution-time quota dies mid-flight and its output is lost.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
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
		Long: "Polls the MOST RECENT run of a Phantom, which is why it needs no container\n" +
			"id — convenient straight after a launch, and wrong once several runs\n" +
			"overlap or when an older run is the subject; use `container output` with\n" +
			"an explicit id there. --from-pos resumes from a position: echo back\n" +
			"`data.output_pos` from the previous response so each poll returns only new\n" +
			"console lines. `data.is_running` turning false is the stop signal.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "Takes the AGENT id, not a container id, and stops every container that\n" +
			"Phantom currently has running — there is no way to abort one of several\n" +
			"selectively. Console output written before the stop stays readable through\n" +
			"`container output`, but execution time already consumed is not returned to\n" +
			"the quota.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
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
