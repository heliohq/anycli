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
		Use:   "list",
		Short: "List subscribers (GET /subscribers)",
		Long: "Cursor-paged, not page-numbered: take `meta.next_cursor` from the response\n" +
			"and pass it as --cursor, since there is no --page here. --status narrows\n" +
			"to active, unsubscribed, unconfirmed, bounced or junk. --include accepts\n" +
			"only the value `groups`, which folds each subscriber's group membership\n" +
			"into the same response and saves one call per person.",
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
		Use:   "get <id-or-email>",
		Short: "Get a subscriber by id or email (GET /subscribers/{id|email})",
		Long: "The single argument may be either a subscriber id or the email address\n" +
			"itself, so an address does not have to be resolved to an id first. This is\n" +
			"the only place in the tool where an email substitutes for an id:\n" +
			"`subscriber update`, `subscriber delete`, `group assign` and the rest all\n" +
			"require the numeric id this returns.",
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
		Use:   "create",
		Short: "Create or upsert a subscriber (POST /subscribers)",
		Long: "An upsert, not a strict create: a new address answers 201 and an address\n" +
			"already on file answers 200 with the existing record updated, so this\n" +
			"never fails as a duplicate. --groups is additive here, taking a\n" +
			"comma-separated list of group ids. --fields is a JSON object whose keys\n" +
			"must already exist as custom fields — an unknown key is not created\n" +
			"implicitly, so run `field list` or `field create` first. Bringing back\n" +
			"someone who previously opted out needs --resubscribe; without it their\n" +
			"unsubscribed status stands.",
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
		Use:   "update <id>",
		Short: "Update a subscriber (PUT /subscribers/{id})",
		Long: "Only the flags actually passed are sent, so unmentioned attributes survive\n" +
			"— with one destructive exception: --groups REPLACES the whole membership\n" +
			"set, removing the subscriber from every group not named in the list.\n" +
			"Adding one group without disturbing the others is `group assign`. Takes\n" +
			"the id, not an email; resolve one with `subscriber get`.",
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
		Use:   "delete <id>",
		Short: "Delete a subscriber (DELETE /subscribers/{id})",
		Long: "Ordinary removal from the account. This is NOT the GDPR erasure — for a\n" +
			"right-to-be-forgotten request use `subscriber forget`, which destroys the\n" +
			"personal data rather than the record. Deleting does not suppress the\n" +
			"address: re-adding it later with `subscriber create` succeeds.",
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
		Use:   "count",
		Short: "Count subscribers (GET /subscribers?limit=0)",
		Long: "The same endpoint as `subscriber list` requested with a page size of zero,\n" +
			"so the total arrives in the response envelope with no subscriber rows\n" +
			"attached — one call instead of paging the account. It takes no filters, so\n" +
			"the number covers every status; a per-status count means `subscriber list\n" +
			"--status <s>` and reading the envelope's total.",
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
		Use:   "activity <id>",
		Short: "Subscriber activity log (GET /subscribers/{id}/activity-log)",
		Long: "Page-numbered with --page and defaulting to 100 per page, unlike the\n" +
			"cursor-paged `subscriber list` — the two do not share a pagination model.\n" +
			"--log-name narrows to one kind of event, such as campaign_send, email_open\n" +
			"or link_click, which is usually necessary since an engaged subscriber\n" +
			"accumulates a long undifferentiated log.",
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
		Use:   "forget <id>",
		Short: "GDPR-forget a subscriber (POST /subscribers/{id}/forget)",
		Long: "A permanent erasure of the person's data for a right-to-be-forgotten\n" +
			"request, not a soft delete and not reversible by re-creating the address.\n" +
			"Everyday removal is `subscriber delete`. Reach for this only when the\n" +
			"request is explicitly a GDPR one.",
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
