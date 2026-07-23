package mailerlite

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newSubscriberCmd builds the `mailerlite subscriber` command tree — the core
// CRM-of-email surface: list/get/create/update/delete plus count, activity, and
// GDPR forget.
func (s *Service) newSubscriberCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "subscriber", Short: "Subscribers (list, get, create, update, delete, count, activity, forget)"}
	cmd.AddCommand(
		s.newSubscriberListCmd(token),
		s.newSubscriberGetCmd(token),
		s.newSubscriberCreateCmd(token),
		s.newSubscriberUpdateCmd(token),
		s.newSubscriberDeleteCmd(token),
		s.newSubscriberCountCmd(token),
		s.newSubscriberActivityCmd(token),
		s.newSubscriberForgetCmd(token),
	)
	return cmd
}

func (s *Service) newSubscriberListCmd(token string) *cobra.Command {
	var status, cursor, include string
	var limit int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List subscribers (GET /subscribers)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if status != "" {
				q.Set("filter[status]", status)
			}
			if include != "" {
				q.Set("include", include)
			}
			setLimitCursor(cmd, q, limit, cursor)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/subscribers", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "filter by status: active|unsubscribed|unconfirmed|bounced|junk")
	cmd.Flags().IntVar(&limit, "limit", 25, "page size (default 25)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor from a prior response's meta.next_cursor")
	cmd.Flags().StringVar(&include, "include", "", "include related data (only 'groups' is supported)")
	return cmd
}

func (s *Service) newSubscriberGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <id-or-email>",
		Short:       "Get a subscriber by id or email (GET /subscribers/{id|email})",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/subscribers/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newSubscriberCreateCmd(token string) *cobra.Command {
	var email, fields, groups, status string
	var resubscribe bool
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create or upsert a subscriber (POST /subscribers)",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := subscriberWriteBody(cmd, email, fields, groups, status, resubscribe)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/subscribers", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "subscriber email (required)")
	cmd.Flags().StringVar(&fields, "fields", "", "custom fields as a JSON object")
	cmd.Flags().StringVar(&groups, "groups", "", "comma-separated group ids to add the subscriber to")
	cmd.Flags().StringVar(&status, "status", "", "subscriber status: active|unsubscribed|unconfirmed|bounced|junk")
	cmd.Flags().BoolVar(&resubscribe, "resubscribe", false, "re-subscribe an unsubscribed subscriber")
	_ = cmd.MarkFlagRequired("email")
	return cmd
}

func (s *Service) newSubscriberUpdateCmd(token string) *cobra.Command {
	var fields, groups, status string
	cmd := &cobra.Command{
		Use:         "update <id>",
		Short:       "Update a subscriber (PUT /subscribers/{id})",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := subscriberWriteBody(cmd, "", fields, groups, status, false)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPut, "/subscribers/"+url.PathEscape(args[0]), nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&fields, "fields", "", "custom fields as a JSON object")
	cmd.Flags().StringVar(&groups, "groups", "", "comma-separated group ids (subscriber is removed from unlisted groups)")
	cmd.Flags().StringVar(&status, "status", "", "subscriber status: active|unsubscribed|unconfirmed|bounced|junk")
	return cmd
}

// subscriberWriteBody assembles a create/update body from the shared flags:
// email (create only), custom fields (JSON object), groups (id list), status,
// and resubscribe. Only flags the caller set are included.
func subscriberWriteBody(cmd *cobra.Command, email, fields, groups, status string, resubscribe bool) (map[string]any, error) {
	pairs := []bodyField{
		{key: "email", value: email, set: cmd.Flags().Changed("email")},
		{key: "status", value: status, set: cmd.Flags().Changed("status")},
		{key: "resubscribe", value: resubscribe, set: cmd.Flags().Changed("resubscribe")},
	}
	if cmd.Flags().Changed("fields") {
		v, err := decodeJSONFlag("fields", fields)
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, bodyField{key: "fields", value: v, set: true})
	}
	if cmd.Flags().Changed("groups") {
		pairs = append(pairs, bodyField{key: "groups", value: splitList(groups), set: true})
	}
	return buildBody(pairs), nil
}

func (s *Service) newSubscriberDeleteCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "delete <id>",
		Short:       "Delete a subscriber (DELETE /subscribers/{id})",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodDelete, "/subscribers/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newSubscriberCountCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "count",
		Short:       "Count subscribers (GET /subscribers?limit=0)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{"limit": {"0"}}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/subscribers", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newSubscriberActivityCmd(token string) *cobra.Command {
	var logName string
	var limit, page int
	cmd := &cobra.Command{
		Use:         "activity <id>",
		Short:       "Subscriber activity log (GET /subscribers/{id}/activity-log)",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			if logName != "" {
				q.Set("filter[log_name]", logName)
			}
			setLimitPage(cmd, q, limit, page)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/subscribers/"+url.PathEscape(args[0])+"/activity-log", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&logName, "log-name", "", "filter by log name, e.g. campaign_send|email_open|link_click")
	cmd.Flags().IntVar(&limit, "limit", 100, "page size (default 100)")
	cmd.Flags().IntVar(&page, "page", 1, "page number (starts at 1)")
	return cmd
}

func (s *Service) newSubscriberForgetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "forget <id>",
		Short:       "GDPR-forget a subscriber (POST /subscribers/{id}/forget)",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/subscribers/"+url.PathEscape(args[0])+"/forget", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}
