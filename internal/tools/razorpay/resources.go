package razorpay

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// resourceDef describes one Razorpay gateway resource that is exposed as a
// `list`/`get` command pair. word is the cobra command word (no underscores —
// the design-318 lint forbids them); path is the API collection path, which
// keeps its underscore form (e.g. /payment_links) because that is the provider
// route.
type resourceDef struct {
	word  string
	short string
	path  string
	// listLong and getLong are the `Long` for this resource's two leaves. Both
	// leaves are built by shared constructors, so the prose that separates a
	// settlement from a subscription travels on the spec; the texts are the
	// long* consts below.
	listLong string
	getLong  string
}

// resources is the AI-relevant, read-mostly gateway surface a payments/finance
// teammate reasons over (design §2). Money-moving writes (capture, refund
// create, payment-link create, order create) are intentionally deferred to a
// second pass and are not registered here. RazorpayX banking (payouts) is a
// separate, higher-risk scope family and is out of this tool's first scope.
var resources = []resourceDef{
	{word: "payment", short: "Payments", path: "/payments", listLong: longPaymentList, getLong: longPaymentGet},
	{word: "order", short: "Orders", path: "/orders", listLong: longOrderList, getLong: longOrderGet},
	{word: "refund", short: "Refunds", path: "/refunds", listLong: longRefundList, getLong: longRefundGet},
	{word: "customer", short: "Customers", path: "/customers", listLong: longCustomerList, getLong: longCustomerGet},
	{word: "payment-link", short: "Payment links", path: "/payment_links", listLong: longPaymentLinkList, getLong: longPaymentLinkGet},
	{word: "settlement", short: "Settlements", path: "/settlements", listLong: longSettlementList, getLong: longSettlementGet},
	{word: "subscription", short: "Subscriptions", path: "/subscriptions", listLong: longSubscriptionList, getLong: longSubscriptionGet},
}

// newResourceCmd builds the `<resource>` group with its `list` and `get`
// leaves. The group carries no side-effect annotation (design-318: only
// runnable leaves are annotated).
func (s *Service) newResourceCmd(token string, r resourceDef) *cobra.Command {
	group := &cobra.Command{
		Use:   r.word,
		Short: r.short,
	}
	group.AddCommand(s.newListCmd(token, r), s.newGetCmd(token, r))
	return group
}

func (s *Service) newListCmd(token string, r resourceDef) *cobra.Command {
	var p listParams
	cmd := &cobra.Command{
		Use:         "list",
		Short:       fmt.Sprintf("List %s", r.short),
		Long:        r.listLong,
		Annotations: map[string]string{sideEffectAnnotation: "false"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, r.path, p.query(cmd))
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	p.register(cmd)
	return cmd
}

func (s *Service) newGetCmd(token string, r resourceDef) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "get <id>",
		Short:       fmt.Sprintf("Fetch one %s by id", r.word),
		Long:        r.getLong,
		Annotations: map[string]string{sideEffectAnnotation: "false"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := strings.TrimSpace(args[0])
			if id == "" {
				return &usageError{msg: fmt.Sprintf("razorpay %s get: id is required", r.word)}
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, r.path+"/"+url.PathEscape(id), nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	return cmd
}
