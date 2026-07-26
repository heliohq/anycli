package contacts

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// contactGroup is the subset of the ContactGroup resource the summary renders.
type contactGroup struct {
	ResourceName  string `json:"resourceName"`
	Name          string `json:"name"`
	FormattedName string `json:"formattedName"`
	GroupType     string `json:"groupType"`
	MemberCount   int    `json:"memberCount"`
}

func (g *contactGroup) label() string {
	if g.FormattedName != "" {
		return g.FormattedName
	}
	return g.Name
}

func (s *Service) newGroupsListCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List contact groups (contactGroups.list)",
		Long: "Contact groups are what the Google Contacts UI calls labels. Each entry\n" +
			"carries a member count and a groupType separating Google's system groups\n" +
			"(myContacts, starred, chatBuddies) from user-created ones. This lists\n" +
			"the groups, not their members; nothing in this tool enumerates who is in\n" +
			"a group or puts anyone into one.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/contactGroups", nil, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				ContactGroups []contactGroup `json:"contactGroups"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("contacts: decode contact groups: %w", err)
			}
			if len(resp.ContactGroups) == 0 {
				fmt.Fprintln(s.stdout(), "no contact groups")
				return nil
			}
			for _, g := range resp.ContactGroups {
				fmt.Fprintf(s.stdout(), "%s\t%s\t(%d members)\n", g.ResourceName, g.label(), g.MemberCount)
			}
			return nil
		},
	}
}

func (s *Service) newGroupsGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <resource-name>",
		Short: "Show one contact group (contactGroups.get)",
		Long: "Takes the group's resource name as `groups list` prints it —\n" +
			"contactGroups/family for a system group, contactGroups/<opaque-id> for a\n" +
			"user-created label. The request deliberately asks for zero members, so\n" +
			"the response gives the group's name, type and member COUNT and never the\n" +
			"member list: this cannot answer who is in a group.",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("maxMembers", "0")
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/"+args[0], q, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var g contactGroup
			if err := json.Unmarshal(body, &g); err != nil {
				return fmt.Errorf("contacts: decode contact group: %w", err)
			}
			fmt.Fprintf(s.stdout(), "Name:         %s\nResourceName: %s\nGroupType:    %s\nMembers:      %d\n",
				g.label(), g.ResourceName, g.GroupType, g.MemberCount)
			return nil
		},
	}
}
