package omnisend

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newEventCmd builds the `event` group: fire a customer event that triggers an
// Omnisend automation workflow.
func (s *Service) newEventCmd(token string) *cobra.Command {
	cmd := newGroupCmd("event", "Customer events (send)")
	cmd.AddCommand(s.newEventSendCmd(token))
	return cmd
}

func (s *Service) newEventSendCmd(token string) *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send a customer event (POST /events). --data is the raw event JSON body.",
		Long: "This is the ONLY way to start an Omnisend automation from here — there\n" +
			"is no command that runs a workflow directly, so firing the event a\n" +
			"workflow listens for is the trigger path. --data names the event with\n" +
			"`eventName`, identifies the person under `contact` (by email or phone),\n" +
			"and puts anything the workflow branches on under `properties`. An event\n" +
			"whose name no workflow is listening for is accepted and silently does\n" +
			"nothing.",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := decodeJSONFlag("data", data)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/events", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "raw event JSON body (contact identifier + event fields)")
	_ = cmd.MarkFlagRequired("data")
	return cmd
}

// newCampaignCmd builds the read-only `campaign` group: list and inspect
// campaigns ("did the promo go out?").
func (s *Service) newCampaignCmd(token string) *cobra.Command {
	cmd := newGroupCmd("campaign", "Campaigns (list, get)")
	cmd.AddCommand(
		s.newCampaignListCmd(token),
		s.newCampaignGetCmd(token),
	)
	return cmd
}

func (s *Service) newCampaignListCmd(token string) *cobra.Command {
	var limit int
	var after string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List campaigns (GET /campaigns)",
		Long: "Read-only surface: campaigns are authored and sent in the Omnisend UI\n" +
			"and there is no command here that creates, edits or triggers one. Use\n" +
			"this to answer what exists and what state it is in, then `campaign get`\n" +
			"for one campaign's detail. Page with --limit and --after.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			applyListQuery(q, limit, after)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/campaigns", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerListFlags(cmd, &limit, &after)
	return cmd
}

func (s *Service) newCampaignGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a campaign by id (GET /campaigns/{id})",
		Long: "--id is the campaign id from `campaign list`. Returns the campaign\n" +
			"record including its send status, which is how to answer whether a promo\n" +
			"actually went out. Nothing in this tool resolves delivery for one\n" +
			"recipient — the campaign record is as fine-grained as it gets.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/campaigns/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "campaign id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

// newSegmentCmd builds the `segment` group: build and inspect audience slices.
func (s *Service) newSegmentCmd(token string) *cobra.Command {
	cmd := newGroupCmd("segment", "Segments (list, get, create)")
	cmd.AddCommand(
		s.newSegmentListCmd(token),
		s.newSegmentGetCmd(token),
		s.newSegmentCreateCmd(token),
	)
	return cmd
}

func (s *Service) newSegmentListCmd(token string) *cobra.Command {
	var limit int
	var after string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List segments (GET /segments)",
		Long: "Segments are saved filters that Omnisend evaluates when they are used,\n" +
			"not frozen contact lists, so the same segment can cover different people\n" +
			"in two campaigns sent a week apart. No command enumerates a segment's\n" +
			"members; `contact list` is the only way to walk the audience.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			applyListQuery(q, limit, after)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/segments", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerListFlags(cmd, &limit, &after)
	return cmd
}

func (s *Service) newSegmentGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a segment by id (GET /segments/{id})",
		Long: "--id is the segment id from `segment list`. The response includes the\n" +
			"segment's filter definition, which is the same nested shape\n" +
			"`segment create --data` expects — read a working segment before\n" +
			"authoring a new filter rather than inventing the grammar.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/segments/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "segment id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newSegmentCreateCmd(token string) *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a segment (POST /segments). --data is the raw segment JSON body.",
		Long: "--data carries a `name` plus the nested `filter` object in Omnisend's own\n" +
			"filter grammar, which is not validated here — a syntactically valid body\n" +
			"with a nonsense filter is rejected by the API, not at parse time. There\n" +
			"is no update or delete command, so a wrong segment stays until someone\n" +
			"removes it in the Omnisend UI.",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := decodeJSONFlag("data", data)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/segments", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "raw segment JSON body")
	_ = cmd.MarkFlagRequired("data")
	return cmd
}

// newProductCmd builds the read-only `product` group: catalog data campaigns
// and automations reference.
func (s *Service) newProductCmd(token string) *cobra.Command {
	cmd := newGroupCmd("product", "Products (list, get)")
	cmd.AddCommand(
		s.newProductListCmd(token),
		s.newProductGetCmd(token),
	)
	return cmd
}

func (s *Service) newProductListCmd(token string) *cobra.Command {
	var limit int
	var after string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List products (GET /products)",
		Long: "The catalog Omnisend holds for the connected store, which is what\n" +
			"campaigns and automations reference in product blocks and abandoned-cart\n" +
			"flows. It mirrors whatever the store synced, so a product missing here\n" +
			"is a store-sync gap rather than a permission problem. Read-only: this\n" +
			"tool cannot create or update catalog entries.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			applyListQuery(q, limit, after)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/products", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerListFlags(cmd, &limit, &after)
	return cmd
}

func (s *Service) newProductGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a product by id (GET /products/{id})",
		Long: "--id is the product id from `product list`. There is no lookup by SKU,\n" +
			"title or store URL, so identifying a product always starts from the\n" +
			"list.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/products/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "product id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

// newBatchCmd builds the `batch` group: bulk upserts (efficiency — avoid N
// single-record calls) plus batch status inspection.
func (s *Service) newBatchCmd(token string) *cobra.Command {
	cmd := newGroupCmd("batch", "Batches (get, create)")
	cmd.AddCommand(
		s.newBatchGetCmd(token),
		s.newBatchCreateCmd(token),
	)
	return cmd
}

func (s *Service) newBatchGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a batch by id (GET /batches/{id})",
		Long: "--id is the id `batch create` returned. Poll this until the batch leaves\n" +
			"its in-progress state; a batch that has finished can still contain\n" +
			"individual items that failed, so the top-level status alone does not\n" +
			"prove every item landed.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/batches/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "batch id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newBatchCreateCmd(token string) *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a batch of bulk operations (POST /batches). --data is the raw batch JSON body.",
		Long: "The bulk path: one --data body naming a `method`, an `endpoint` such as\n" +
			"\"contacts\", and an `items` array replaces one API call per item, which\n" +
			"is what keeps a few thousand contact writes inside rate limits. It runs\n" +
			"ASYNCHRONOUSLY — the response is a batch id and a queued status, not a\n" +
			"result, so the work is not done when this command returns. Poll\n" +
			"`batch get` for the outcome.",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := decodeJSONFlag("data", data)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/batches", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "raw batch JSON body (method, endpoint, items)")
	_ = cmd.MarkFlagRequired("data")
	return cmd
}

// newBrandCmd builds the read-only `brand` group: confirm which Omnisend
// account/store the connection is bound to.
func (s *Service) newBrandCmd(token string) *cobra.Command {
	cmd := newGroupCmd("brand", "Brand/account info (get)")
	cmd.AddCommand(s.newBrandGetCmd(token))
	return cmd
}

func (s *Service) newBrandGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get the current brand/account (GET /brands/current)",
		Long: "Reports which Omnisend brand the connected token is bound to. One cheap\n" +
			"read that answers which store a write will land in — worth doing before\n" +
			"the first write of a session when more than one Omnisend connection\n" +
			"exists, rather than discovering it from a campaign that went to the\n" +
			"wrong audience.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/brands/current", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}
