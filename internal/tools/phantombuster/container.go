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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
