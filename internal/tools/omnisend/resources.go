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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
