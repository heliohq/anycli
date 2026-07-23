package braze

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newTemplatesCmd builds the `templates` resource group. Today it exposes the
// email-template read slice (`templates email list|info`); authoring is out of
// scope for v1.
func (s *Service) newTemplatesCmd(c *client) *cobra.Command {
	group := newGroupCmd("templates", "Message-template discovery (read-only)")
	email := newGroupCmd("email", "Email templates")
	email.AddCommand(
		s.newTemplatesEmailListCmd(c),
		s.newTemplatesEmailInfoCmd(c),
	)
	group.AddCommand(email)
	return group
}

// newTemplatesEmailListCmd is `templates email list` (GET /templates/email/list).
func (s *Service) newTemplatesEmailListCmd(c *client) *cobra.Command {
	var limit, offset int
	var modifiedAfter, modifiedBefore string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List email templates, paginated",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "max templates to return (max 1000)")
	cmd.Flags().IntVar(&offset, "offset", 0, "number of templates to skip")
	cmd.Flags().StringVar(&modifiedAfter, "modified-after", "", "ISO-8601 lower bound on last-modified")
	cmd.Flags().StringVar(&modifiedBefore, "modified-before", "", "ISO-8601 upper bound on last-modified")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		if cmd.Flags().Changed("limit") {
			q.Set("limit", strconv.Itoa(limit))
		}
		if cmd.Flags().Changed("offset") {
			q.Set("offset", strconv.Itoa(offset))
		}
		if modifiedAfter != "" {
			q.Set("modified_after", modifiedAfter)
		}
		if modifiedBefore != "" {
			q.Set("modified_before", modifiedBefore)
		}
		body, err := c.get(cmd.Context(), "/templates/email/list", q)
		if err != nil {
			return err
		}
		return c.emit(body)
	}
	return cmd
}

// newTemplatesEmailInfoCmd is `templates email info` (GET /templates/email/info).
func (s *Service) newTemplatesEmailInfoCmd(c *client) *cobra.Command {
	var templateID string
	cmd := &cobra.Command{
		Use:         "info",
		Short:       "Get an email template's content and metadata",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	cmd.Flags().StringVar(&templateID, "template-id", "", "email template identifier (required)")
	_ = cmd.MarkFlagRequired("template-id")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		q.Set("email_template_id", templateID)
		body, err := c.get(cmd.Context(), "/templates/email/info", q)
		if err != nil {
			return err
		}
		return c.emit(body)
	}
	return cmd
}

// newContentBlocksCmd builds the `content-blocks` resource group: list / info
// (read-only discovery of reusable Content Blocks).
func (s *Service) newContentBlocksCmd(c *client) *cobra.Command {
	group := newGroupCmd("content-blocks", "Content Block discovery (read-only)")
	group.AddCommand(
		s.newContentBlocksListCmd(c),
		s.newContentBlocksInfoCmd(c),
	)
	return group
}

// newContentBlocksListCmd is `content-blocks list` (GET /content_blocks/list).
func (s *Service) newContentBlocksListCmd(c *client) *cobra.Command {
	var limit, offset int
	var modifiedAfter, modifiedBefore string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List Content Blocks, paginated",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "max Content Blocks to return (max 1000)")
	cmd.Flags().IntVar(&offset, "offset", 0, "number of Content Blocks to skip")
	cmd.Flags().StringVar(&modifiedAfter, "modified-after", "", "ISO-8601 lower bound on last-modified")
	cmd.Flags().StringVar(&modifiedBefore, "modified-before", "", "ISO-8601 upper bound on last-modified")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		if cmd.Flags().Changed("limit") {
			q.Set("limit", strconv.Itoa(limit))
		}
		if cmd.Flags().Changed("offset") {
			q.Set("offset", strconv.Itoa(offset))
		}
		if modifiedAfter != "" {
			q.Set("modified_after", modifiedAfter)
		}
		if modifiedBefore != "" {
			q.Set("modified_before", modifiedBefore)
		}
		body, err := c.get(cmd.Context(), "/content_blocks/list", q)
		if err != nil {
			return err
		}
		return c.emit(body)
	}
	return cmd
}

// newContentBlocksInfoCmd is `content-blocks info` (GET /content_blocks/info).
func (s *Service) newContentBlocksInfoCmd(c *client) *cobra.Command {
	var contentBlockID string
	cmd := &cobra.Command{
		Use:         "info",
		Short:       "Get a Content Block's content and metadata",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	cmd.Flags().StringVar(&contentBlockID, "content-block-id", "", "Content Block identifier (required)")
	_ = cmd.MarkFlagRequired("content-block-id")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		q.Set("content_block_id", contentBlockID)
		body, err := c.get(cmd.Context(), "/content_blocks/info", q)
		if err != nil {
			return err
		}
		return c.emit(body)
	}
	return cmd
}
