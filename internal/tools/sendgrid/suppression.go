package sendgrid

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// longSuppressionBounces, longSuppressionUnsubscribes and longSuppressionBlocks
// are the three suppression Longs. They live next to the shared builder because
// the builder is what fixes the endpoint each one describes, and the three
// lists mean genuinely different things despite the identical flag surface.
const (
	longSuppressionBounces = "A bounce is the receiving server rejecting the message; SendGrid then\n" +
		"suppresses further sends to that address until it is removed, so mail\n" +
		"that vanishes for one recipient while working for others usually ends\n" +
		"up explained here. Each entry carries the raw SMTP `reason`, which is\n" +
		"what separates a hard bounce (no such mailbox) from a soft one (mailbox\n" +
		"full, greylisted). --limit defaults to 100 and --offset pages."

	longSuppressionUnsubscribes = "The GLOBAL unsubscribe list: addresses that opted out of everything from\n" +
		"this account. Suppression-group (ASM) opt-outs live in a separate store\n" +
		"that this tool does not reach, so an address absent here can still be\n" +
		"unsubscribed from the group a given send uses. --limit defaults to 100\n" +
		"and --offset pages."

	longSuppressionBlocks = "A block is the receiving server refusing the message for a reputation or\n" +
		"policy reason rather than a bad mailbox, so unlike a bounce the address\n" +
		"itself is usually fine and the sending domain or IP is the problem.\n" +
		"--limit defaults to 100 and --offset pages."
)

func (s *Service) newSuppressionCmd(token string, region *string) *cobra.Command {
	cmd := &cobra.Command{Use: "suppression", Short: "Suppression lists (bounces, unsubscribes, blocks)"}
	cmd.AddCommand(
		s.newSuppressionListCmd(token, region, "bounces", "/suppression/bounces", "List bounced addresses (GET /v3/suppression/bounces)", longSuppressionBounces),
		s.newSuppressionListCmd(token, region, "unsubscribes", "/suppression/unsubscribes", "List global unsubscribes (GET /v3/suppression/unsubscribes)", longSuppressionUnsubscribes),
		s.newSuppressionListCmd(token, region, "blocks", "/suppression/blocks", "List blocked addresses (GET /v3/suppression/blocks)", longSuppressionBlocks),
	)
	return cmd
}

// newSuppressionListCmd builds one suppression subcommand. The three lists share
// the same paged GET shape (limit/offset), differing only in path.
func (s *Service) newSuppressionListCmd(token string, region *string, use, path, short, long string) *cobra.Command {
	var limit, offset int
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Long:        long,
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("limit", intToString(limit))
			q.Set("offset", intToString(offset))
			resp, err := s.call(cmd.Context(), token, *region, http.MethodGet, path, q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 100, "max entries to return")
	cmd.Flags().IntVar(&offset, "offset", 0, "pagination offset")
	return cmd
}
