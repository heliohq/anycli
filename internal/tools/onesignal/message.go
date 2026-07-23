package onesignal

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// messageSendFlags holds the flags for `message send`. Exactly one targeting
// method must be provided (OneSignal: "Only one targeting method is allowed per
// message"); the rule is enforced client-side and fails exit 2 before any HTTP.
type messageSendFlags struct {
	channel        string
	segments       []string
	subscriptionID []string
	emails         []string
	phones         []string
	filters        string
	heading        string
	content        string
	name           string
	sendAfter      string
}

func (s *Service) newMessageSendCmd(key, appID string) *cobra.Command {
	f := &messageSendFlags{}
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send a push / email / SMS message (POST /notifications)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := f.buildBody(appID)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), key, http.MethodPost, "/notifications", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&f.channel, "channel", "push", "delivery channel: push|email|sms")
	cmd.Flags().StringArrayVar(&f.segments, "segment", nil, "target an included segment by name (repeatable)")
	cmd.Flags().StringArrayVar(&f.subscriptionID, "subscription-id", nil, "target a subscription id (repeatable)")
	cmd.Flags().StringArrayVar(&f.emails, "email", nil, "target an email address (repeatable)")
	cmd.Flags().StringArrayVar(&f.phones, "phone", nil, "target an E.164 phone number (repeatable)")
	cmd.Flags().StringVar(&f.filters, "filters", "", "target by a JSON array of OneSignal filters")
	cmd.Flags().StringVar(&f.heading, "heading", "", "message title / email subject")
	cmd.Flags().StringVar(&f.content, "content", "", "message body (required)")
	cmd.Flags().StringVar(&f.name, "name", "", "internal message name (optional)")
	cmd.Flags().StringVar(&f.sendAfter, "send-after", "", "schedule delivery for this timestamp (optional)")
	return cmd
}

// buildBody assembles the POST /notifications payload, enforcing the
// exactly-one-targeting-method rule and mapping content per channel.
func (f *messageSendFlags) buildBody(appID string) (map[string]any, error) {
	switch f.channel {
	case "push", "email", "sms":
	default:
		return nil, &usageError{msg: "--channel must be one of push|email|sms"}
	}
	if f.content == "" {
		return nil, &usageError{msg: "--content is required"}
	}

	body := map[string]any{
		"app_id":         appID,
		"target_channel": f.channel,
	}
	if err := f.applyTargeting(body); err != nil {
		return nil, err
	}
	f.applyContent(body)
	if f.name != "" {
		body["name"] = f.name
	}
	if f.sendAfter != "" {
		body["send_after"] = f.sendAfter
	}
	return body, nil
}

// applyTargeting sets exactly one targeting field on body, or returns a usage
// error when zero or more than one targeting method is supplied.
func (f *messageSendFlags) applyTargeting(body map[string]any) error {
	type target struct {
		field string
		set   bool
		apply func()
	}
	targets := []target{
		{"included_segments", len(f.segments) > 0, func() { body["included_segments"] = f.segments }},
		{"include_subscription_ids", len(f.subscriptionID) > 0, func() { body["include_subscription_ids"] = f.subscriptionID }},
		{"email_to", len(f.emails) > 0, func() { body["email_to"] = f.emails }},
		{"include_phone_numbers", len(f.phones) > 0, func() { body["include_phone_numbers"] = f.phones }},
		{"filters", f.filters != "", nil},
	}
	chosen := -1
	for i, t := range targets {
		if !t.set {
			continue
		}
		if chosen != -1 {
			return &usageError{msg: "only one targeting method is allowed per message (use exactly one of --segment, --subscription-id, --email, --phone, --filters)"}
		}
		chosen = i
	}
	if chosen == -1 {
		return &usageError{msg: "a targeting method is required (use one of --segment, --subscription-id, --email, --phone, --filters)"}
	}
	if targets[chosen].field == "filters" {
		v, err := decodeJSONFlag("filters", f.filters)
		if err != nil {
			return err
		}
		body["filters"] = v
		return nil
	}
	targets[chosen].apply()
	return nil
}

// applyContent maps --heading/--content onto the channel-correct fields: email
// uses email_subject/email_body; push and sms use headings/contents (sms
// ignores headings).
func (f *messageSendFlags) applyContent(body map[string]any) {
	if f.channel == "email" {
		body["email_body"] = f.content
		if f.heading != "" {
			body["email_subject"] = f.heading
		}
		return
	}
	body["contents"] = map[string]string{"en": f.content}
	if f.channel == "push" && f.heading != "" {
		body["headings"] = map[string]string{"en": f.heading}
	}
}

func (s *Service) newMessageListCmd(key, appID string) *cobra.Command {
	var limit, offset int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recent messages (GET /notifications)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := appQuery(appID)
			if cmd.Flags().Changed("limit") {
				q.Set("limit", intToString(limit))
			}
			if cmd.Flags().Changed("offset") {
				q.Set("offset", intToString(offset))
			}
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/notifications", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "max messages to return (server max/default 50)")
	cmd.Flags().IntVar(&offset, "offset", 0, "result offset for pagination")
	return cmd
}

func (s *Service) newMessageGetCmd(key, appID string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "View one message's delivery stats (GET /notifications/{id})",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if id == "" {
				return &usageError{msg: "--id is required"}
			}
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/notifications/"+url.PathEscape(id), appQuery(appID), nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "notification id (required)")
	return cmd
}

func (s *Service) newMessageCancelCmd(key, appID string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "cancel",
		Short: "Cancel a scheduled message (DELETE /notifications/{id})",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if id == "" {
				return &usageError{msg: "--id is required"}
			}
			resp, err := s.call(cmd.Context(), key, http.MethodDelete, "/notifications/"+url.PathEscape(id), appQuery(appID), nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "notification id (required)")
	return cmd
}
