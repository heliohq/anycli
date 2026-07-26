package novu

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newWorkflowCmd is the read-only `workflow` group over /v2/workflows: discover
// the trigger identifiers `event trigger` needs.
func (s *Service) newWorkflowCmd(c *client) *cobra.Command {
	group := newGroupCmd("workflow", "Discover workflows (read-only)")
	group.AddCommand(
		s.newWorkflowListCmd(c),
		s.newWorkflowGetCmd(c),
	)
	return group
}

// The workflow Longs, grouped above the constructors that use them because
// both leaves are built by the shared leafCmd helper.
const (
	longWorkflowList = "The catalog of what can be triggered, and the source of the trigger\n" +
		"identifier `event trigger --workflow` requires. Read each workflow's\n" +
		"status: an inactive workflow still accepts a trigger and answers\n" +
		"trigger_not_active while delivering nothing. Read-only — workflows are\n" +
		"authored in the Novu dashboard and nothing here creates or edits one.\n" +
		"Paged with --limit and --offset."

	longWorkflowGet = "--workflow-id accepts either the workflow id or its trigger identifier.\n" +
		"The step list in the response is what determines which channels a trigger\n" +
		"can possibly reach, so this is where a no_workflow_active_steps_defined\n" +
		"outcome — which the trigger only names — is actually explained."
)

func (s *Service) newWorkflowListCmd(c *client) *cobra.Command {
	var query, status, tags, orderBy, orderDirection string
	var limit, offset int
	cmd := leafCmd("list", "List workflows", longWorkflowList, readOnly, func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		addQueryString(q, "query", query)
		addQueryString(q, "status", status)
		addQueryString(q, "tags", tags)
		addQueryString(q, "orderBy", orderBy)
		addQueryString(q, "orderDirection", orderDirection)
		addQueryInt(q, "limit", limit)
		addQueryInt(q, "offset", offset)
		out, err := c.call(cmd.Context(), http.MethodGet, "/v2/workflows", q, nil)
		if err != nil {
			return err
		}
		return s.emit(out)
	})
	f := cmd.Flags()
	f.StringVar(&query, "query", "", "search query")
	f.StringVar(&status, "status", "", "filter by status")
	f.StringVar(&tags, "tags", "", "filter by tags")
	f.StringVar(&orderBy, "order-by", "", "field to order by")
	f.StringVar(&orderDirection, "order-direction", "", "ASC or DESC")
	f.IntVar(&limit, "limit", 0, "max results per page")
	f.IntVar(&offset, "offset", 0, "results offset")
	return cmd
}

func (s *Service) newWorkflowGetCmd(c *client) *cobra.Command {
	var id string
	cmd := leafCmd("get", "Get one workflow by id", longWorkflowGet, readOnly, func(cmd *cobra.Command, _ []string) error {
		if err := requireFlag("workflow-id", id); err != nil {
			return err
		}
		out, err := c.call(cmd.Context(), http.MethodGet, "/v2/workflows/"+pathEscape(id), nil, nil)
		if err != nil {
			return err
		}
		return s.emit(out)
	})
	cmd.Flags().StringVar(&id, "workflow-id", "", "workflow id or trigger identifier (required)")
	return cmd
}
