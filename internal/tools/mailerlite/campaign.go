package mailerlite

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newCampaignCmd builds the `mailerlite campaign` command tree — the
// draft → schedule → check-report loop.
func (s *Service) newCampaignCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "campaign", Short: "Campaigns (list, get, create, update, schedule, cancel, delete, report)"}
	cmd.AddCommand(
		s.newCampaignListCmd(token),
		s.newCampaignGetCmd(token),
		s.newCampaignDataCmd(token, "create", "Create a campaign (POST /campaigns)", longCampaignCreate, http.MethodPost, ""),
		s.newCampaignDataCmd(token, "update", "Update a campaign (PUT /campaigns/{id})", longCampaignUpdate, http.MethodPut, ""),
		s.newCampaignDataCmd(token, "schedule", "Schedule a campaign (POST /campaigns/{id}/schedule)", longCampaignSchedule, http.MethodPost, "/schedule"),
		s.newCampaignActionCmd(token, "cancel", "Cancel a scheduled campaign (POST /campaigns/{id}/cancel)", longCampaignCancel, http.MethodPost, "/cancel"),
		s.newCampaignActionCmd(token, "delete", "Delete a campaign (DELETE /campaigns/{id})", longCampaignDelete, http.MethodDelete, ""),
		s.newCampaignReportCmd(token),
	)
	return cmd
}

func (s *Service) newCampaignListCmd(token string) *cobra.Command {
	var status, campaignType string
	var limit, page int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List campaigns (GET /campaigns)",
		Long: "Page-numbered with --page. --status is sent, draft or ready — `ready`\n" +
			"means scheduled and still cancellable, which is the only window `campaign\n" +
			"cancel` works in. --type is regular, ab, resend or rss.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if status != "" {
				q.Set("filter[status]", status)
			}
			if campaignType != "" {
				q.Set("filter[type]", campaignType)
			}
			setLimitPage(cmd, q, limit, page)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/campaigns", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "filter by status: sent|draft|ready")
	cmd.Flags().StringVar(&campaignType, "type", "", "filter by type: regular|ab|resend|rss")
	cmd.Flags().IntVar(&limit, "limit", 25, "page size (default 25)")
	cmd.Flags().IntVar(&page, "page", 1, "page number (starts at 1)")
	return cmd
}

func (s *Service) newCampaignGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get a campaign (GET /campaigns/{id})",
		Long: "Returns the whole campaign including its nested email blocks and delivery\n" +
			"configuration. Since `campaign create` and `campaign update` take raw\n" +
			"JSON, reading an existing campaign here is the practical way to learn the\n" +
			"exact payload shape those flags expect.",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/campaigns/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

// longCampaignCreate, longCampaignUpdate and longCampaignSchedule are the
// raw-body Longs, and longCampaignCancel / longCampaignDelete the bodyless
// ones. They live next to the two shared builders because it is the builder
// that fixes the --data contract and the id argument they describe.
const (
	longCampaignCreate = "--data is the entire campaign as one JSON object — name, type, the\n" +
		"`emails` array carrying subject, from address and content, and the groups\n" +
		"to send to — because the payload nests email blocks that do not reduce to\n" +
		"flat flags. Reading an existing campaign with `campaign get` is the\n" +
		"practical way to learn the exact shape. Creating only produces a DRAFT:\n" +
		"nothing is sent until `campaign schedule` runs."

	longCampaignUpdate = "--data names the fields to change on an existing campaign, as one JSON\n" +
		"object in the same shape `campaign create` takes. Only a campaign that has\n" +
		"not gone out yet can be edited — a sent campaign is immutable, and\n" +
		"changing its content means creating a new one."

	longCampaignSchedule = "This is the command that actually sends. --data carries the delivery\n" +
		"configuration: `{\"delivery\":\"instant\"}` goes out immediately to every\n" +
		"subscriber the campaign targets, while a future send carries its own date,\n" +
		"time and timezone fields. An instant delivery cannot be called back —\n" +
		"`campaign cancel` only reaches a send that is still pending. Confirm the\n" +
		"audience with `campaign get` before running this."

	longCampaignCancel = "Only reaches a campaign that is scheduled and still waiting; a send\n" +
		"already in flight or completed cannot be recalled. Cancelling returns the\n" +
		"campaign to an editable state rather than deleting it, so it can be fixed\n" +
		"and scheduled again."

	longCampaignDelete = "Removes the campaign along with its reporting, so `campaign report` and\n" +
		"the aggregate metrics for that send are gone too. Deleting a sent campaign\n" +
		"does not unsend it — the mail is already delivered; only the record\n" +
		"disappears. To stop a pending send instead, use `campaign cancel`."
)

// newCampaignDataCmd builds a write command whose body is supplied as raw JSON
// via --data (the campaign create/update/schedule payloads carry nested email
// blocks and delivery config that resist flat flags). create takes no id; the
// others take an id and append suffix to the path.
func (s *Service) newCampaignDataCmd(token, use, short, long, method, suffix string) *cobra.Command {
	var data string
	takesID := use != "create"
	args := cobra.NoArgs
	if takesID {
		args = cobra.ExactArgs(1)
	}
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Long:        long,
		Annotations: writeAction,
		Args:        args,
		RunE: func(cmd *cobra.Command, cmdArgs []string) error {
			body, err := decodeJSONFlag("data", data)
			if err != nil {
				return err
			}
			path := "/campaigns"
			if takesID {
				path += "/" + url.PathEscape(cmdArgs[0]) + suffix
			}
			resp, err := s.call(cmd.Context(), token, method, path, nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	if takesID {
		cmd.Use = use + " <id>"
	}
	cmd.Flags().StringVar(&data, "data", "", "request body as a JSON object (required)")
	_ = cmd.MarkFlagRequired("data")
	return cmd
}

// newCampaignActionCmd builds a bodyless action (cancel/delete) keyed on the
// campaign id.
func (s *Service) newCampaignActionCmd(token, use, short, long, method, suffix string) *cobra.Command {
	return &cobra.Command{
		Use:         use + " <id>",
		Short:       short,
		Long:        long,
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, method, "/campaigns/"+url.PathEscape(args[0])+suffix, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newCampaignReportCmd(token string) *cobra.Command {
	var limit, page int
	cmd := &cobra.Command{
		Use:   "report <id>",
		Short: "Campaign subscriber-activity report (GET /campaigns/{id}/reports/subscriber-activity)",
		Long: "Per-subscriber engagement — who opened and who clicked — rather than the\n" +
			"aggregate totals `campaign get` carries. Page-numbered at 25 per page by\n" +
			"default, so a large send runs to many pages; raise --limit before walking\n" +
			"them.",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			setLimitPage(cmd, q, limit, page)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/campaigns/"+url.PathEscape(args[0])+"/reports/subscriber-activity", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 25, "page size (default 25)")
	cmd.Flags().IntVar(&page, "page", 1, "page number (starts at 1)")
	return cmd
}
