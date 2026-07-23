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
		Use:         "list",
		Short:       "List subscribers (one page; use --after to continue)",
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
		Use:         "get <id>",
		Short:       "Show one subscriber",
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
		Use:         "create",
		Short:       "Create or upsert a subscriber",
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
		Use:         "update <id>",
		Short:       "Update a subscriber's attributes",
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
		Use:         "unsubscribe <id>",
		Short:       "Unsubscribe a subscriber",
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
