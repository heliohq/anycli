package kit

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// broadcastCmd groups the broadcast (newsletter) commands — the highest-value
// teammate action: draft/schedule newsletters and read open/click stats.
func (s *Service) broadcastCmd(token string) *cobra.Command {
	group := newGroupCmd("broadcast", "Draft, schedule, and report on broadcasts")
	group.AddCommand(
		s.broadcastListCmd(token),
		s.broadcastGetCmd(token),
		s.broadcastCreateCmd(token),
		s.broadcastUpdateCmd(token),
		s.broadcastStatsCmd(token),
	)
	return group
}

func (s *Service) broadcastListCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List broadcasts (one page; use --after to continue)",
		Long: "Drafts, scheduled and already-sent newsletters all come back together, so\n" +
			"the send state has to be read off each item rather than filtered for. One\n" +
			"page per call; continue with --after.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	lf := registerListFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		lf.apply(q)
		body, err := s.call(cmd.Context(), token, http.MethodGet, "/broadcasts", q, nil)
		if err != nil {
			return err
		}
		return s.emitData(body, "broadcasts")
	}
	return cmd
}

func (s *Service) broadcastGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Show one broadcast",
		Long: "The whole broadcast including its HTML content, its scheduled send time\n" +
			"and the subscriber filter deciding who receives it. Reading this before\n" +
			"scheduling is the only way to confirm the audience, since `broadcast\n" +
			"create` reports back what was sent rather than how many people it resolves\n" +
			"to.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/broadcasts/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emitData(body, "broadcast")
		},
	}
}

func (s *Service) broadcastCreateCmd(token string) *cobra.Command {
	var subject, content, description, sendAt, publishedAt, previewText string
	var public bool
	var tagID, segmentID int
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a broadcast (draft unless --send-at is set)",
		Long: "Omitting --send-at leaves a DRAFT that will never go out on its own;\n" +
			"supplying a future ISO8601 --send-at schedules it, and from then on it\n" +
			"sends without further confirmation. --subject and --content are both\n" +
			"required and --content is HTML, not markdown. --description defaults to\n" +
			"the subject.\n" +
			"\n" +
			"--tag-id and --segment-id restrict recipients, but they are combined as an\n" +
			"ANY group: passing both sends to everyone in either the tag or the\n" +
			"segment, which widens the audience rather than narrowing it to the\n" +
			"overlap. Passing neither sends to the whole list.\n" +
			"\n" +
			"--public governs publication to the web newsletter feed only and has\n" +
			"nothing to do with email delivery.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if subject == "" || content == "" {
				return &usageError{msg: "--subject and --content are required"}
			}
			payload := broadcastPayload(subject, content, description, sendAt, publishedAt, previewText, public, tagID, segmentID)
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/broadcasts", nil, payload)
			if err != nil {
				return err
			}
			return s.emitData(body, "broadcast")
		},
	}
	cmd.Flags().StringVar(&subject, "subject", "", "email subject line (required)")
	cmd.Flags().StringVar(&content, "content", "", "email HTML content (required)")
	cmd.Flags().StringVar(&description, "description", "", "internal description (defaults to subject)")
	cmd.Flags().StringVar(&sendAt, "send-at", "", "ISO8601 scheduled send time; omit to save a draft")
	cmd.Flags().StringVar(&publishedAt, "published-at", "", "ISO8601 display timestamp")
	cmd.Flags().StringVar(&previewText, "preview-text", "", "inbox preview text")
	cmd.Flags().BoolVar(&public, "public", false, "publish to the web newsletter feed")
	cmd.Flags().IntVar(&tagID, "tag-id", 0, "restrict recipients to a tag (subscriber_filter)")
	cmd.Flags().IntVar(&segmentID, "segment-id", 0, "restrict recipients to a segment (subscriber_filter)")
	return cmd
}

func (s *Service) broadcastUpdateCmd(token string) *cobra.Command {
	var subject, content, description, sendAt, publishedAt, previewText string
	var public bool
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a broadcast's fields",
		Long: "Only the flags passed are sent; at least one is required. Setting\n" +
			"--send-at on a draft is what schedules it, and changing it again is the\n" +
			"only way to reschedule — there is no separate send or cancel verb. A\n" +
			"broadcast that has already gone out cannot be recalled by editing it. Note\n" +
			"this command has no --tag-id or --segment-id: the recipient filter can\n" +
			"only be set at creation.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{}
			setIfNonEmpty(payload, "subject", subject)
			setIfNonEmpty(payload, "content", content)
			setIfNonEmpty(payload, "description", description)
			setIfNonEmpty(payload, "send_at", sendAt)
			setIfNonEmpty(payload, "published_at", publishedAt)
			setIfNonEmpty(payload, "preview_text", previewText)
			if cmd.Flags().Changed("public") {
				payload["public"] = public
			}
			if len(payload) == 0 {
				return &usageError{msg: "nothing to update: set at least one field flag"}
			}
			body, err := s.call(cmd.Context(), token, http.MethodPut, "/broadcasts/"+url.PathEscape(args[0]), nil, payload)
			if err != nil {
				return err
			}
			return s.emitData(body, "broadcast")
		},
	}
	cmd.Flags().StringVar(&subject, "subject", "", "new subject line")
	cmd.Flags().StringVar(&content, "content", "", "new HTML content")
	cmd.Flags().StringVar(&description, "description", "", "new internal description")
	cmd.Flags().StringVar(&sendAt, "send-at", "", "new ISO8601 scheduled send time")
	cmd.Flags().StringVar(&publishedAt, "published-at", "", "new ISO8601 display timestamp")
	cmd.Flags().StringVar(&previewText, "preview-text", "", "new inbox preview text")
	cmd.Flags().BoolVar(&public, "public", false, "publish to the web newsletter feed")
	return cmd
}

func (s *Service) broadcastStatsCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "stats <id>",
		Short: "Show open/click stats for a broadcast",
		Long: "Opens, clicks and recipient counts for this one send, which is the level\n" +
			"`account stats --email` cannot give — that reports the account-wide rate\n" +
			"across everything. Only meaningful once a broadcast has actually been\n" +
			"sent.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/broadcasts/"+url.PathEscape(args[0])+"/stats", nil, nil)
			if err != nil {
				return err
			}
			return s.emitData(body, "broadcast")
		},
	}
}

// broadcastPayload assembles the create body. description defaults to the
// subject; a tag or segment id becomes an `any` subscriber_filter group.
func broadcastPayload(subject, content, description, sendAt, publishedAt, previewText string, public bool, tagID, segmentID int) map[string]any {
	if description == "" {
		description = subject
	}
	payload := map[string]any{
		"subject":     subject,
		"content":     content,
		"description": description,
		"public":      public,
	}
	setIfNonEmpty(payload, "send_at", sendAt)
	setIfNonEmpty(payload, "published_at", publishedAt)
	setIfNonEmpty(payload, "preview_text", previewText)
	if group := subscriberFilterGroup(tagID, segmentID); group != nil {
		payload["subscriber_filter"] = []any{group}
	}
	return payload
}

// subscriberFilterGroup builds an `any` filter group from an optional tag and
// segment id, or nil when neither is set.
func subscriberFilterGroup(tagID, segmentID int) map[string]any {
	var clauses []any
	if tagID > 0 {
		clauses = append(clauses, map[string]any{"type": "tag", "ids": []int{tagID}})
	}
	if segmentID > 0 {
		clauses = append(clauses, map[string]any{"type": "segment", "ids": []int{segmentID}})
	}
	if len(clauses) == 0 {
		return nil
	}
	return map[string]any{"any": clauses}
}

// setIfNonEmpty writes key=value into m only when value is non-empty.
func setIfNonEmpty(m map[string]any, key, value string) {
	if value != "" {
		m[key] = value
	}
}
