package activecampaign

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// listFlags registers the shared pagination + filter passthrough flags on a
// list command and returns accessors bound to the command.
func registerListFlags(cmd *cobra.Command) {
	cmd.Flags().Int("limit", 0, "max rows to return (provider default 20, max 100)")
	cmd.Flags().Int("offset", 0, "row offset for pagination")
	cmd.Flags().StringArray("query", nil, "extra query param key=value (repeatable), e.g. --query email=a@b.com --query 'filters[status]=1'")
}

// buildListQuery assembles limit/offset/repeatable --query into url.Values.
func buildListQuery(cmd *cobra.Command) (url.Values, error) {
	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")
	queries, _ := cmd.Flags().GetStringArray("query")
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}
	for _, kv := range queries {
		i := strings.IndexByte(kv, '=')
		if i < 0 {
			return nil, &usageError{msg: fmt.Sprintf("--query %q must be key=value", kv)}
		}
		q.Add(kv[:i], kv[i+1:])
	}
	return q, nil
}

// buildInner assembles a v3 resource object from convenience flag values (empty
// values dropped) then merges the optional --data JSON object over them.
func buildInner(convenience map[string]string, dataJSON string) (map[string]any, error) {
	inner := map[string]any{}
	for k, v := range convenience {
		if strings.TrimSpace(v) != "" {
			inner[k] = v
		}
	}
	if strings.TrimSpace(dataJSON) != "" {
		var extra map[string]any
		if err := json.Unmarshal([]byte(dataJSON), &extra); err != nil {
			return nil, &usageError{msg: fmt.Sprintf("--data is not a JSON object: %v", err)}
		}
		for k, v := range extra {
			inner[k] = v
		}
	}
	return inner, nil
}

// --- contact commands ---

func (s *Service) newContactListCmd(c *client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List/search contacts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q, err := buildListQuery(cmd)
			if err != nil {
				return err
			}
			return c.get(cmd.Context(), "contacts", q)
		},
	}
	registerListFlags(cmd)
	return cmd
}

func (s *Service) newContactGetCmd(c *client) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Retrieve one contact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.get(cmd.Context(), "contacts/"+url.PathEscape(args[0]), nil)
		},
	}
}

func (s *Service) newContactCreateCmd(c *client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a contact",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			inner, err := contactInnerFromFlags(cmd)
			if err != nil {
				return err
			}
			return c.send(cmd.Context(), http.MethodPost, "contacts", map[string]any{"contact": inner})
		},
	}
	registerContactFieldFlags(cmd)
	return cmd
}

func (s *Service) newContactUpdateCmd(c *client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a contact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			inner, err := contactInnerFromFlags(cmd)
			if err != nil {
				return err
			}
			return c.send(cmd.Context(), http.MethodPut, "contacts/"+url.PathEscape(args[0]), map[string]any{"contact": inner})
		},
	}
	registerContactFieldFlags(cmd)
	return cmd
}

func (s *Service) newContactDeleteCmd(c *client) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a contact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.send(cmd.Context(), http.MethodDelete, "contacts/"+url.PathEscape(args[0]), nil)
		},
	}
}

func registerContactFieldFlags(cmd *cobra.Command) {
	cmd.Flags().String("email", "", "contact email")
	cmd.Flags().String("first-name", "", "contact first name")
	cmd.Flags().String("last-name", "", "contact last name")
	cmd.Flags().String("phone", "", "contact phone")
	cmd.Flags().String("data", "", "extra contact fields as a JSON object (merged over the flags)")
}

func contactInnerFromFlags(cmd *cobra.Command) (map[string]any, error) {
	email, _ := cmd.Flags().GetString("email")
	first, _ := cmd.Flags().GetString("first-name")
	last, _ := cmd.Flags().GetString("last-name")
	phone, _ := cmd.Flags().GetString("phone")
	data, _ := cmd.Flags().GetString("data")
	return buildInner(map[string]string{
		"email":     email,
		"firstName": first,
		"lastName":  last,
		"phone":     phone,
	}, data)
}

func (s *Service) newContactSubscribeCmd(c *client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "subscribe",
		Short: "Add/remove a contact to a list (status 1=subscribe, 2=unsubscribe)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			list, _ := cmd.Flags().GetString("list")
			contact, _ := cmd.Flags().GetString("contact")
			status, _ := cmd.Flags().GetString("status")
			if list == "" || contact == "" {
				return &usageError{msg: "--list and --contact are required"}
			}
			body := map[string]any{"contactList": map[string]any{"list": list, "contact": contact, "status": status}}
			return c.send(cmd.Context(), http.MethodPost, "contactLists", body)
		},
	}
	cmd.Flags().String("list", "", "list id (required)")
	cmd.Flags().String("contact", "", "contact id (required)")
	cmd.Flags().String("status", "1", "1 to subscribe, 2 to unsubscribe")
	return cmd
}

func (s *Service) newContactTagCmd(c *client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tag",
		Short: "Apply a tag to a contact",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			contact, _ := cmd.Flags().GetString("contact")
			tag, _ := cmd.Flags().GetString("tag")
			if contact == "" || tag == "" {
				return &usageError{msg: "--contact and --tag are required"}
			}
			body := map[string]any{"contactTag": map[string]any{"contact": contact, "tag": tag}}
			return c.send(cmd.Context(), http.MethodPost, "contactTags", body)
		},
	}
	cmd.Flags().String("contact", "", "contact id (required)")
	cmd.Flags().String("tag", "", "tag id (required)")
	return cmd
}

func (s *Service) newContactUntagCmd(c *client) *cobra.Command {
	return &cobra.Command{
		Use:   "untag <contactTagId>",
		Short: "Remove a tag from a contact (by the contactTag association id)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.send(cmd.Context(), http.MethodDelete, "contactTags/"+url.PathEscape(args[0]), nil)
		},
	}
}

func (s *Service) newContactAutomateCmd(c *client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "automate",
		Short: "Enroll a contact into an automation",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			contact, _ := cmd.Flags().GetString("contact")
			automation, _ := cmd.Flags().GetString("automation")
			if contact == "" || automation == "" {
				return &usageError{msg: "--contact and --automation are required"}
			}
			body := map[string]any{"contactAutomation": map[string]any{"contact": contact, "automation": automation}}
			return c.send(cmd.Context(), http.MethodPost, "contactAutomations", body)
		},
	}
	cmd.Flags().String("contact", "", "contact id (required)")
	cmd.Flags().String("automation", "", "automation id (required)")
	return cmd
}

// --- tag create ---

func (s *Service) newTagCreateCmd(c *client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a segmentation tag",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			name, _ := cmd.Flags().GetString("name")
			tagType, _ := cmd.Flags().GetString("type")
			desc, _ := cmd.Flags().GetString("description")
			if name == "" {
				return &usageError{msg: "--name is required"}
			}
			inner := map[string]any{"tag": name, "tagType": tagType}
			if desc != "" {
				inner["description"] = desc
			}
			return c.send(cmd.Context(), http.MethodPost, "tags", map[string]any{"tag": inner})
		},
	}
	cmd.Flags().String("name", "", "tag name (required)")
	cmd.Flags().String("type", "contact", "tag type: contact or template")
	cmd.Flags().String("description", "", "tag description")
	return cmd
}

// --- generic read/write helpers ---

// newSimpleListCmd builds a `list` command over a plain v3 collection resource
// (contacts→"contacts", lists→"lists", dealGroups, …) with pagination + filter
// passthrough.
func (s *Service) newSimpleListCmd(c *client, resource string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List " + resource,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q, err := buildListQuery(cmd)
			if err != nil {
				return err
			}
			return c.get(cmd.Context(), resource, q)
		},
	}
	registerListFlags(cmd)
	return cmd
}

// newSimpleGetCmd builds a `get <id>` command over a plain v3 resource.
func (s *Service) newSimpleGetCmd(c *client, resource string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Retrieve one " + strings.TrimSuffix(resource, "s"),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.get(cmd.Context(), resource+"/"+url.PathEscape(args[0]), nil)
		},
	}
}

// newResourceCreateCmd builds a `create` command that wraps --data under the
// singular resource key (deal → {"deal": …}) and POSTs to the collection.
func (s *Service) newResourceCreateCmd(c *client, wrapper, resource string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a " + wrapper,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, _ := cmd.Flags().GetString("data")
			inner, err := buildInner(nil, data)
			if err != nil {
				return err
			}
			return c.send(cmd.Context(), http.MethodPost, resource, map[string]any{wrapper: inner})
		},
	}
	cmd.Flags().String("data", "", "the "+wrapper+" fields as a JSON object")
	return cmd
}

// newResourceUpdateCmd builds an `update <id>` command that wraps --data under
// the singular resource key and PUTs to the resource.
func (s *Service) newResourceUpdateCmd(c *client, wrapper, resource string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a " + wrapper,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, _ := cmd.Flags().GetString("data")
			inner, err := buildInner(nil, data)
			if err != nil {
				return err
			}
			return c.send(cmd.Context(), http.MethodPut, resource+"/"+url.PathEscape(args[0]), map[string]any{wrapper: inner})
		},
	}
	cmd.Flags().String("data", "", "the "+wrapper+" fields as a JSON object")
	return cmd
}
