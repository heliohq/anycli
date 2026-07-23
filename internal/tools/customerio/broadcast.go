package customerio

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newBroadcastListCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:         "list",
		Short:       "List broadcasts (GET /v1/broadcasts)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd, key, http.MethodGet, "/v1/broadcasts", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newBroadcastGetCmd(key string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get a broadcast (GET /v1/broadcasts/{id})",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd, key, http.MethodGet, "/v1/broadcasts/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "broadcast id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newBroadcastMetricsCmd(key string) *cobra.Command {
	var id string
	var m metricsParams
	cmd := &cobra.Command{
		Use:         "metrics",
		Short:       "Broadcast performance metrics (GET /v1/broadcasts/{id}/metrics)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			m.apply(q)
			resp, err := s.call(cmd, key, http.MethodGet, "/v1/broadcasts/"+url.PathEscape(id)+"/metrics", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "broadcast id")
	registerMetricsFlags(cmd, &m)
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newBroadcastTriggerCmd(key string) *cobra.Command {
	var id, data, perUserData, dataFileURL string
	var emails, ids []string
	cmd := &cobra.Command{
		Use:         "trigger",
		Short:       "Trigger an API broadcast (POST /v1/campaigns/{id}/triggers). Rate limit: 1 request / 10s per broadcast",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// At most one audience selector may be given; none is valid too
			// (the broadcast's own audience/segment applies).
			selectors := 0
			for _, set := range []bool{len(emails) > 0, len(ids) > 0, perUserData != "", dataFileURL != ""} {
				if set {
					selectors++
				}
			}
			if selectors > 1 {
				return &usageError{msg: "at most one of --emails, --ids, --per-user-data, --data-file-url may be set"}
			}
			body := map[string]any{}
			if data != "" {
				v, err := decodeJSONFlag("data", data)
				if err != nil {
					return err
				}
				body["data"] = v
			}
			switch {
			case len(emails) > 0:
				body["emails"] = emails
			case len(ids) > 0:
				body["ids"] = ids
			case perUserData != "":
				v, err := decodeJSONFlag("per-user-data", perUserData)
				if err != nil {
					return err
				}
				body["per_user_data"] = v
			case dataFileURL != "":
				body["data_file_url"] = dataFileURL
			}
			resp, err := s.call(cmd, key, http.MethodPost, "/v1/campaigns/"+url.PathEscape(id)+"/triggers", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "broadcast id")
	cmd.Flags().StringVar(&data, "data", "", "liquid merge data as raw JSON object")
	cmd.Flags().StringSliceVar(&emails, "emails", nil, "audience by email (comma-separated or repeatable)")
	cmd.Flags().StringSliceVar(&ids, "ids", nil, "audience by customer id (comma-separated or repeatable)")
	cmd.Flags().StringVar(&perUserData, "per-user-data", "", "per-recipient merge data as raw JSON array")
	cmd.Flags().StringVar(&dataFileURL, "data-file-url", "", "URL of a data file describing the audience")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newBroadcastStatusCmd(key string) *cobra.Command {
	var id, trigger string
	var errorsOnly bool
	cmd := &cobra.Command{
		Use:         "status",
		Short:       "Trigger status (GET /v1/campaigns/{id}/triggers/{trigger}) or per-recipient errors (/errors)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path := "/v1/campaigns/" + url.PathEscape(id) + "/triggers/" + url.PathEscape(trigger)
			if errorsOnly {
				path += "/errors"
			}
			resp, err := s.call(cmd, key, http.MethodGet, path, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "broadcast id")
	cmd.Flags().StringVar(&trigger, "trigger", "", "trigger id")
	cmd.Flags().BoolVar(&errorsOnly, "errors", false, "return per-recipient trigger errors")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("trigger")
	return cmd
}
