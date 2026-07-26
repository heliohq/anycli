package kit

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// subscriberCmd groups the subscriber (contact list) commands.
func (s *Service) subscriberCmd(token string) *cobra.Command {
	group := newGroupCmd("subscriber", "Manage subscribers (the contact list)")
	group.AddCommand(
		s.subscriberListCmd(token),
		s.subscriberGetCmd(token),
		s.subscriberCreateCmd(token),
		s.subscriberUpdateCmd(token),
		s.subscriberUnsubscribeCmd(token),
	)
	return group
}

func (s *Service) subscriberListCmd(token string) *cobra.Command {
	var status, emailAddress, createdAfter, createdBefore string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List subscribers (one page; use --after to continue)",
		Long: "Kit filters to ACTIVE subscribers unless --status says otherwise, so\n" +
			"bounced, complained and cancelled records are invisible on a bare call —\n" +
			"pass --status all to see the whole list. --email is an exact lookup and is\n" +
			"the way to resolve an address to the numeric id every other subscriber\n" +
			"verb needs. --created-after and --created-before take ISO8601 timestamps.\n" +
			"One page per call; continue with --after.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	lf := registerListFlags(cmd)
	cmd.Flags().StringVar(&status, "status", "", "active|inactive|bounced|complained|cancelled|all")
	cmd.Flags().StringVar(&emailAddress, "email", "", "exact email lookup")
	cmd.Flags().StringVar(&createdAfter, "created-after", "", "ISO8601 lower bound on created_at")
	cmd.Flags().StringVar(&createdBefore, "created-before", "", "ISO8601 upper bound on created_at")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		if status != "" {
			q.Set("status", status)
		}
		if emailAddress != "" {
			q.Set("email_address", emailAddress)
		}
		if createdAfter != "" {
			q.Set("created_after", createdAfter)
		}
		if createdBefore != "" {
			q.Set("created_before", createdBefore)
		}
		lf.apply(q)
		body, err := s.call(cmd.Context(), token, http.MethodGet, "/subscribers", q, nil)
		if err != nil {
			return err
		}
		return s.emitData(body, "subscribers")
	}
	return cmd
}

func (s *Service) subscriberGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Show one subscriber",
		Long: "Takes the numeric subscriber id. There is no lookup by email address here\n" +
			"— `subscriber list --email <address>` is the resolver, and its result\n" +
			"carries the id this needs.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/subscribers/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emitData(body, "subscriber")
		},
	}
}

func (s *Service) subscriberCreateCmd(token string) *cobra.Command {
	var email, firstName, state string
	var fields map[string]string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create or upsert a subscriber",
		Long: "An upsert keyed on --email: an address already on the list comes back\n" +
			"updated rather than duplicated, so this is safe to re-run. --fields takes\n" +
			"repeatable key=value pairs whose keys must already exist as custom fields\n" +
			"— check `custom-field list` first, since an unrecognised key is not\n" +
			"created implicitly. --state is active or inactive.\n" +
			"\n" +
			"Note that adding someone this way runs no form logic and triggers no\n" +
			"automation; entering a subscriber into a funnel is `form add` or `tag\n" +
			"add`.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if email == "" {
				return &usageError{msg: "--email is required"}
			}
			payload := map[string]any{"email_address": email}
			if firstName != "" {
				payload["first_name"] = firstName
			}
			if state != "" {
				payload["state"] = state
			}
			if len(fields) > 0 {
				custom := map[string]any{}
				for k, v := range fields {
					custom[k] = v
				}
				payload["fields"] = custom
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/subscribers", nil, payload)
			if err != nil {
				return err
			}
			return s.emitData(body, "subscriber")
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "subscriber email address (required)")
	cmd.Flags().StringVar(&firstName, "first-name", "", "subscriber first name")
	cmd.Flags().StringVar(&state, "state", "", "active|inactive")
	cmd.Flags().StringToStringVar(&fields, "fields", nil, "custom field values, key=value")
	return cmd
}

func (s *Service) subscriberUpdateCmd(token string) *cobra.Command {
	var email, firstName string
	var fields map[string]string
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a subscriber's attributes",
		Long: "Only the flags actually passed are sent, and --fields merges the custom\n" +
			"fields it names while leaving the rest alone. At least one flag is\n" +
			"required. Changing --email rewrites the identity the whole account keys\n" +
			"on, including how `subscriber list --email` and every membership command\n" +
			"find this person. Unlike `subscriber create` there is no --state here.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{}
			if email != "" {
				payload["email_address"] = email
			}
			if firstName != "" {
				payload["first_name"] = firstName
			}
			if len(fields) > 0 {
				custom := map[string]any{}
				for k, v := range fields {
					custom[k] = v
				}
				payload["fields"] = custom
			}
			if len(payload) == 0 {
				return &usageError{msg: "nothing to update: set --email, --first-name, or --fields"}
			}
			body, err := s.call(cmd.Context(), token, http.MethodPut, "/subscribers/"+url.PathEscape(args[0]), nil, payload)
			if err != nil {
				return err
			}
			return s.emitData(body, "subscriber")
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "new email address")
	cmd.Flags().StringVar(&firstName, "first-name", "", "new first name")
	cmd.Flags().StringToStringVar(&fields, "fields", nil, "custom field values, key=value")
	return cmd
}

func (s *Service) subscriberUnsubscribeCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "unsubscribe <id>",
		Short: "Unsubscribe a subscriber",
		Long: "Marks the subscriber inactive rather than deleting them: the record, its\n" +
			"tags and its history all survive and stop receiving mail. There is no undo\n" +
			"verb here. The id must be a positive integer and is checked locally, so an\n" +
			"email address passed by mistake fails before any request goes out.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil || id <= 0 {
				return &usageError{msg: "subscriber id must be a positive integer"}
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost,
				"/subscribers/"+strconv.Itoa(id)+"/unsubscribe", nil, nil)
			if err != nil {
				return err
			}
			return s.emitData(body, "subscriber")
		},
	}
}
