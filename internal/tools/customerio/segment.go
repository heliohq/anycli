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
		Long: "One unpaginated response covering both manual segments and the data-driven\n" +
			"ones defined in the Customer.io UI. Only manual segments can be created or\n" +
			"deleted here; a data-driven segment's membership is computed from its\n" +
			"conditions and cannot be edited through this tool at all.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "Three endpoints behind one verb, and --count and --used-by are mutually\n" +
			"exclusive. --count returns just the member count, which is one request\n" +
			"instead of paging every member through `segment members`. --used-by names\n" +
			"the campaigns and newsletters that depend on this segment and is the check\n" +
			"to run before `segment delete`, since deleting a segment something still\n" +
			"targets breaks that automation silently.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "Creates an empty MANUAL segment — a container whose membership is set\n" +
			"explicitly rather than computed from conditions. Nothing in this tool can\n" +
			"then add a person to it: membership writes belong to the Track API, which\n" +
			"is out of scope here, so a segment created this way stays empty until it\n" +
			"is populated elsewhere. Conditional, self-maintaining segments are built\n" +
			"in the Customer.io UI, not here.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
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
		Long: "Irreversible and not confirmable after the fact: the API answers 204 with\n" +
			"no body, so the command prints its own receipt rather than provider JSON.\n" +
			"Run `segment get --used-by` first — a campaign or newsletter still\n" +
			"targeting the segment loses its audience without erroring.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
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
		Long: "Pages the people in the segment with --start and --limit. For size alone\n" +
			"this is the expensive path — `segment get --count` answers in one request.\n" +
			"The reverse lookup, which segments one person belongs to, is `person\n" +
			"segments`.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
