package braze

import (
	"net/url"

	"github.com/spf13/cobra"
)

// newMessagesCmd builds the `messages` resource group: send (immediate),
// schedule (future), and scheduled-list (upcoming). All are act verbs and
// permission-gated by the REST key's scope.
func (s *Service) newMessagesCmd(c *client) *cobra.Command {
	group := newGroupCmd("messages", "Send, schedule, and list scheduled messages")
	group.AddCommand(
		s.newMessagesSendCmd(c),
		s.newMessagesScheduleCmd(c),
		s.newMessagesScheduledListCmd(c),
	)
	return group
}

// newMessagesSendCmd is `messages send` (POST /messages/send): send an
// immediate message. The full, versioned messages / recipients / audience body
// is passed through --body verbatim; the tool only assembles auth and host.
func (s *Service) newMessagesSendCmd(c *client) *cobra.Command {
	var bodyFlag string
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send an immediate message (permission-gated)",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().StringVar(&bodyFlag, "body", "", "raw JSON object: messages, recipients, broadcast, audience, … (required)")
	_ = cmd.MarkFlagRequired("body")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		payload, err := objectBodyFlag("body", bodyFlag, nil)
		if err != nil {
			return err
		}
		body, err := c.post(cmd.Context(), "/messages/send", payload)
		if err != nil {
			return err
		}
		return c.emit(body)
	}
	return cmd
}

// newMessagesScheduleCmd is `messages schedule` (POST
// /messages/schedule/create): schedule a message for the future. --body carries
// the message/recipients object; --schedule carries the schedule object (time /
// in_local_time / at_optimal_time), overlaid onto the body's `schedule` field.
func (s *Service) newMessagesScheduleCmd(c *client) *cobra.Command {
	var bodyFlag, scheduleFlag string
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Schedule a message for the future (permission-gated)",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().StringVar(&bodyFlag, "body", "", "raw JSON object: messages, recipients, broadcast, audience, … (required)")
	cmd.Flags().StringVar(&scheduleFlag, "schedule", "", "raw JSON schedule object: time, in_local_time, at_optimal_time (required)")
	_ = cmd.MarkFlagRequired("body")
	_ = cmd.MarkFlagRequired("schedule")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		schedule, err := decodeJSONFlag("schedule", scheduleFlag)
		if err != nil {
			return err
		}
		payload, err := objectBodyFlag("body", bodyFlag, map[string]any{"schedule": schedule})
		if err != nil {
			return err
		}
		body, err := c.post(cmd.Context(), "/messages/schedule/create", payload)
		if err != nil {
			return err
		}
		return c.emit(body)
	}
	return cmd
}

// newMessagesScheduledListCmd is `messages scheduled-list` (GET
// /messages/scheduled_broadcasts): upcoming scheduled campaigns / Canvases up
// to --end-time.
func (s *Service) newMessagesScheduledListCmd(c *client) *cobra.Command {
	var endTime string
	cmd := &cobra.Command{
		Use:   "scheduled-list",
		Short: "List upcoming scheduled broadcasts",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().StringVar(&endTime, "end-time", "", "ISO-8601 upper bound on scheduled time (required)")
	_ = cmd.MarkFlagRequired("end-time")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		q.Set("end_time", endTime)
		body, err := c.get(cmd.Context(), "/messages/scheduled_broadcasts", q)
		if err != nil {
			return err
		}
		return c.emit(body)
	}
	return cmd
}
