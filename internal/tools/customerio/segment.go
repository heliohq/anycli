package customerio

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func (s *Service) newSegmentListCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List segments (GET /v1/segments)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd, key, http.MethodGet, "/v1/segments", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newSegmentGetCmd(key string) *cobra.Command {
	var id string
	var count, usedBy bool
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a segment (GET /v1/segments/{id}), its size (/customer_count), or dependencies (/used_by)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if count && usedBy {
				return &usageError{msg: "--count and --used-by are mutually exclusive"}
			}
			path := "/v1/segments/" + url.PathEscape(id)
			switch {
			case count:
				path += "/customer_count"
			case usedBy:
				path += "/used_by"
			}
			resp, err := s.call(cmd, key, http.MethodGet, path, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "segment id")
	cmd.Flags().BoolVar(&count, "count", false, "return the segment's member count (/customer_count)")
	cmd.Flags().BoolVar(&usedBy, "used-by", false, "return campaigns/newsletters using the segment (/used_by)")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newSegmentCreateCmd(key string) *cobra.Command {
	var name, description string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a manual segment (POST /v1/segments)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			segment := map[string]any{"name": name}
			if description != "" {
				segment["description"] = description
			}
			resp, err := s.call(cmd, key, http.MethodPost, "/v1/segments", nil, map[string]any{"segment": segment})
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "segment name")
	cmd.Flags().StringVar(&description, "description", "", "segment description")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func (s *Service) newSegmentDeleteCmd(key string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a manual segment (DELETE /v1/segments/{id})",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd, key, http.MethodDelete, "/v1/segments/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			// DELETE returns 204 with an empty body; emit a receipt.
			if len(resp) == 0 {
				return s.emitValue(map[string]any{"ok": true, "deleted": id})
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "segment id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newSegmentMembersCmd(key string) *cobra.Command {
	var id, start string
	var limit int
	cmd := &cobra.Command{
		Use:   "members",
		Short: "List a segment's members (GET /v1/segments/{id}/membership)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if start != "" {
				q.Set("start", start)
			}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			resp, err := s.call(cmd, key, http.MethodGet, "/v1/segments/"+url.PathEscape(id)+"/membership", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "segment id")
	cmd.Flags().StringVar(&start, "start", "", "pagination cursor")
	cmd.Flags().IntVar(&limit, "limit", 0, "max results per page (0 = provider default)")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}
