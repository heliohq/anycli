package wise

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newTransferListCmd monitors outgoing transfers by profile / status / date.
// GET /v1/transfers?profile={id}&status={s}&offset=&limit=&createdDateStart=&createdDateEnd=
func (s *Service) newTransferListCmd(token string) *cobra.Command {
	var profile, status, createdStart, createdEnd string
	var offset, limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List transfers (GET /v1/transfers)",
		Long: "Monitors transfers that already exist — it neither creates nor cancels\n" +
			"them. Every flag is a filter and all are optional, including `--profile`.\n" +
			"`--status` takes Wise's own lower-snake state names, such as\n" +
			"outgoing_payment_sent, and `--created-date-start` / `--created-date-end`\n" +
			"bound creation time, not settlement time. Paging is offset-based: raise\n" +
			"`--offset` by `--limit` for the next page, since no cursor is returned.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if profile != "" {
				q.Set("profile", profile)
			}
			if status != "" {
				q.Set("status", status)
			}
			if createdStart != "" {
				q.Set("createdDateStart", createdStart)
			}
			if createdEnd != "" {
				q.Set("createdDateEnd", createdEnd)
			}
			if cmd.Flags().Changed("offset") {
				q.Set("offset", intToString(offset))
			}
			if cmd.Flags().Changed("limit") {
				q.Set("limit", intToString(limit))
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/v1/transfers", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "profile id filter")
	cmd.Flags().StringVar(&status, "status", "", "transfer status filter (e.g. outgoing_payment_sent)")
	cmd.Flags().StringVar(&createdStart, "created-date-start", "", "ISO-8601 lower bound on creation date")
	cmd.Flags().StringVar(&createdEnd, "created-date-end", "", "ISO-8601 upper bound on creation date")
	cmd.Flags().IntVar(&offset, "offset", 0, "pagination offset")
	cmd.Flags().IntVar(&limit, "limit", 0, "max transfers to return")
	return cmd
}

// newTransferGetCmd reads the status of one transfer.
// GET /v1/transfers/{transferId}
func (s *Service) newTransferGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <transferId>",
		Short: "Get one transfer by id (GET /v1/transfers/{transferId})",
		Long: "Takes no `--profile`: the transfer id from `transfer list` addresses it on\n" +
			"its own. This is the poll target for \"has it landed yet\" — the response\n" +
			"carries the current status plus the rate, fee and recipient the transfer\n" +
			"was created with. Nothing here changes its state.",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet,
				"/v1/transfers/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}
