package gorgias

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newTicketCmd(token, base string) *cobra.Command {
	cmd := newGroupCmd("ticket", "Triage tickets (list, get, create, update)")
	cmd.AddCommand(
		s.newTicketListCmd(token, base),
		s.newTicketGetCmd(token, base),
		s.newTicketCreateCmd(token, base),
		s.newTicketUpdateCmd(token, base),
	)
	return cmd
}

func (s *Service) newTicketListCmd(token, base string) *cobra.Command {
	var page pageFlags
	var view, customer, externalID string
	var trashed bool
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List tickets (GET /tickets)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			page.apply(q)
			if view != "" {
				q.Set("view_id", view)
			}
			if customer != "" {
				q.Set("customer_id", customer)
			}
			if externalID != "" {
				q.Set("external_id", externalID)
			}
			if cmd.Flags().Changed("trashed") {
				q.Set("trashed", boolString(trashed))
			}
			resp, err := s.call(cmd.Context(), token, base, http.MethodGet, "/tickets", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	page.register(cmd)
	cmd.Flags().StringVar(&view, "view", "", "filter to a view's tickets (view id)")
	cmd.Flags().StringVar(&customer, "customer", "", "filter to a customer's tickets (customer id)")
	cmd.Flags().StringVar(&externalID, "external-id", "", "look up a ticket by its foreign-system id")
	cmd.Flags().BoolVar(&trashed, "trashed", false, "include trashed tickets")
	return cmd
}

func (s *Service) newTicketGetCmd(token, base string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <ticket-id>",
		Short:       "Retrieve a ticket (GET /tickets/{id})",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, base, http.MethodGet, "/tickets/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newTicketCreateCmd(token, base string) *cobra.Command {
	var customerEmail, subject, body, channel, via, sourceFrom string
	var sourceTo []string
	var fromAgent bool
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Open a ticket with an initial message (POST /tickets)",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// The initial message's sender references the customer the ticket is
			// for (an incoming, customer-voiced ticket by default).
			message := buildMessage(messageParams{
				channel:     channel,
				via:         via,
				body:        body,
				fromAgent:   fromAgent,
				senderEmail: customerEmail,
				sourceFrom:  sourceFrom,
				sourceTo:    sourceTo,
			})
			// Gorgias requires channel + via + from_agent at the ticket level too.
			payload := map[string]any{
				"channel":    channel,
				"via":        resolveVia(via, channel),
				"from_agent": fromAgent,
				"messages":   []any{message},
			}
			if subject != "" {
				payload["subject"] = subject
			}
			if customerEmail != "" {
				payload["customer"] = map[string]any{"email": customerEmail}
			}
			resp, err := s.call(cmd.Context(), token, base, http.MethodPost, "/tickets", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&customerEmail, "customer-email", "", "email of the customer the ticket is for")
	cmd.Flags().StringVar(&subject, "subject", "", "ticket subject")
	cmd.Flags().StringVar(&body, "body", "", "initial message body (text)")
	cmd.Flags().StringVar(&channel, "channel", "api", "channel: api|email|phone|sms|internal-note")
	cmd.Flags().StringVar(&via, "via", "", "delivery via: api|email|internal-note (default: derived from --channel)")
	cmd.Flags().BoolVar(&fromAgent, "from-agent", false, "the initial message is from an agent (default: from the customer)")
	cmd.Flags().StringVar(&sourceFrom, "source-from", "", "email/phone/sms: sender routing address (email must be a connected Gorgias integration)")
	cmd.Flags().StringArrayVar(&sourceTo, "source-to", nil, "email/phone/sms: recipient routing address (repeatable)")
	_ = cmd.MarkFlagRequired("body")
	return cmd
}

func (s *Service) newTicketUpdateCmd(token, base string) *cobra.Command {
	var status, priority, subject, assignee string
	var tags []string
	cmd := &cobra.Command{
		Use:         "update <ticket-id>",
		Short:       "Update a ticket's status, assignee, priority, or tags (PUT /tickets/{id})",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{}
			if status != "" {
				payload["status"] = status
			}
			if priority != "" {
				payload["priority"] = priority
			}
			if subject != "" {
				payload["subject"] = subject
			}
			if assignee != "" {
				id, err := parseID("assignee", assignee)
				if err != nil {
					return err
				}
				payload["assignee_user"] = map[string]any{"id": id}
			}
			if len(tags) > 0 {
				objs := make([]any, 0, len(tags))
				for _, t := range tags {
					objs = append(objs, map[string]any{"name": t})
				}
				payload["tags"] = objs
			}
			if len(payload) == 0 {
				return &usageError{msg: "gorgias: ticket update needs at least one of --status/--assignee/--priority/--subject/--tag"}
			}
			resp, err := s.call(cmd.Context(), token, base, http.MethodPut, "/tickets/"+url.PathEscape(args[0]), nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "set status: open|closed")
	cmd.Flags().StringVar(&priority, "priority", "", "set priority: critical|high|normal|low")
	cmd.Flags().StringVar(&subject, "subject", "", "set subject")
	cmd.Flags().StringVar(&assignee, "assignee", "", "assign to a user (user id)")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "set a tag by name (repeatable; replaces the ticket's tag set)")
	return cmd
}
