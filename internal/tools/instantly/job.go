package instantly

import (
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newJobCmd(token string) *cobra.Command {
	cmd := newGroupCmd("job", "Background jobs (poll bulk-operation completion)")
	cmd.AddCommand(
		s.newJobListCmd(token),
		s.newJobGetCmd(token),
	)
	return cmd
}

func (s *Service) newJobListCmd(token string) *cobra.Command {
	var page pageFlags
	var status, jobType string
	cmd := &cobra.Command{
		Use:         "list",
		Annotations: readOnly,
		Short:       "List background jobs (GET /background-jobs)",
		Long: "--status (running, for instance) and --type filter; cursor-paged with\n" +
			"--limit and --starting-after. This is how a job id is recovered when it\n" +
			"was not kept from the command that created it — the usual reason a bulk\n" +
			"`lead move` looks like it silently did nothing.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			page.applyQuery(q)
			setIfChanged(cmd, q, "status", "status", status)
			setIfChanged(cmd, q, "type", "type", jobType)
			return s.get(cmd, token, "/background-jobs", q)
		},
	}
	registerPageFlags(cmd, &page)
	cmd.Flags().StringVar(&status, "status", "", "filter by job status")
	cmd.Flags().StringVar(&jobType, "type", "", "filter by job type")
	return cmd
}

func (s *Service) newJobGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         "get",
		Annotations: readOnly,
		Short:       "Get a background job (GET /background-jobs/{id})",
		Long: "--id comes from whatever returned the job, in practice `lead move`. A bulk\n" +
			"operation is not finished when its own command returns, so poll here\n" +
			"until the status stops being running before assuming the leads landed\n" +
			"where they were sent.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.get(cmd, token, "/background-jobs/"+url.PathEscape(id), nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "background job id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}
