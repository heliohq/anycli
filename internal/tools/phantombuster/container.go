package phantombuster

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newContainerListCmd lists all runs (containers) for a Phantom.
// GET /containers/fetch-all?agentId= → {maxLimitReached, containers:[...]}.
func (s *Service) newContainerListCmd(key string) *cobra.Command {
	var agentID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List a Phantom's runs (GET /containers/fetch-all)",
		Long: "--agent-id is required, so run history is always per Phantom and never\n" +
			"workspace-wide. A `maxLimitReached` flag in the response means the history\n" +
			"was TRUNCATED rather than exhausted, and there is no cursor to page\n" +
			"further — an older run is reachable only if its containerId was kept. Each\n" +
			"entry carries the run's status, endType and timestamps.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("agentId", agentID)
			raw, err := s.call(cmd.Context(), key, http.MethodGet, "/containers/fetch-all", q, nil)
			if err != nil {
				return err
			}
			return s.emitObject(raw, nil)
		},
	}
	cmd.Flags().StringVar(&agentID, "agent-id", "", "agent id whose containers to list")
	_ = cmd.MarkFlagRequired("agent-id")
	return cmd
}

// newContainerGetCmd fetches one run by container id.
// GET /containers/fetch?id= → {status, endType, exitCode, resultObject, timestamps, ...}.
func (s *Service) newContainerGetCmd(key string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get one run by container id (GET /containers/fetch)",
		Long: "The authoritative view of one run: `status`, `endType` — why it stopped —\n" +
			"and `exitCode`, with `data.is_running` derived as the poll-loop stop\n" +
			"signal. Terminal is not the same as successful: a run that errored or was\n" +
			"aborted answers here just as cleanly as one that finished, so read\n" +
			"`endType` before spending a call on `container result`.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("id", id)
			raw, err := s.call(cmd.Context(), key, http.MethodGet, "/containers/fetch", q, nil)
			if err != nil {
				return err
			}
			return s.emitObject(raw, nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "container id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

// newContainerOutputCmd fetches a specific container's incremental output.
// GET /containers/fetch-output?id=&fromOutputPos= → {output, outputPos, status, ...}.
func (s *Service) newContainerOutputCmd(key string) *cobra.Command {
	var id string
	var fromPos int
	cmd := &cobra.Command{
		Use:   "output",
		Short: "Poll a specific run's output (GET /containers/fetch-output)",
		Long: "The same incremental console stream as `agent output`, bound to one\n" +
			"explicit container id, which makes it the correct poller when runs overlap\n" +
			"or when inspecting a run that already ended. --from-pos takes the previous\n" +
			"response's `data.output_pos` so each call returns only new lines. This is\n" +
			"the Phantom's own logging — progress and errors — never the extracted\n" +
			"rows.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("id", id)
			if cmd.Flags().Changed("from-pos") {
				q.Set("fromOutputPos", itoa(fromPos))
			}
			raw, err := s.call(cmd.Context(), key, http.MethodGet, "/containers/fetch-output", q, nil)
			if err != nil {
				return err
			}
			return s.emitObject(raw, nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "container id")
	cmd.Flags().IntVar(&fromPos, "from-pos", 0, "resume output from this position (echo data.output_pos)")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

// newContainerResultCmd fetches a run's structured result.
// GET /containers/fetch-result-object?id= → {resultObject: <string|null>}.
func (s *Service) newContainerResultCmd(key string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "result",
		Short: "Fetch a run's structured result object (GET /containers/fetch-result-object)",
		Long: "`data.resultObject` is a JSON STRING, not a nested object — parse it before\n" +
			"reading rows out of it. It is null when the run saved no result, which\n" +
			"includes anything that failed early. For a file a person can download,\n" +
			"PhantomBuster leaves result.csv and result.json in public S3 at\n" +
			"phantombuster.s3.amazonaws.com/<org s3Folder>/<agent s3Folder>/result.csv,\n" +
			"the two folders coming from `org get` and `agent get`; that URL needs no\n" +
			"credentials and this tool does not fetch it.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("id", id)
			raw, err := s.call(cmd.Context(), key, http.MethodGet, "/containers/fetch-result-object", q, nil)
			if err != nil {
				return err
			}
			return s.emitObject(raw, nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "container id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}
