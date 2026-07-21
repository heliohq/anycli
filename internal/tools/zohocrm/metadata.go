package zohocrm

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// newModuleListCmd is `module list` — GET /crm/v8/settings/modules: the
// modules available in this org, including custom ones. The first step before
// picking a --module value.
func (s *Service) newModuleListCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available modules",
		Args:  cobra.NoArgs,
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		body, err := s.call(cmd.Context(), token, http.MethodGet, "/settings/modules", nil)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newFieldListCmd is `field list` — GET /crm/v8/settings/fields?module=M: the
// field API names of a module. Zoho create/update bodies are keyed by field
// API names (Last_Name, not "Last Name"), so this is a hard prerequisite for
// reliable writes.
func (s *Service) newFieldListCmd(token string) *cobra.Command {
	var module string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List a module's field API names",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().StringVar(&module, "module", "", "module API name (required)")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if err := requireModule(module); err != nil {
			return err
		}
		q := url.Values{}
		q.Set("module", strings.TrimSpace(module))
		body, err := s.call(cmd.Context(), token, http.MethodGet, "/settings/fields?"+q.Encode(), nil)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newUserListCmd is `user list` — GET /crm/v8/users[?type=…]. --type filters
// the category (AllUsers, ActiveUsers, CurrentUser, …); omitted, the API
// returns its default set.
func (s *Service) newUserListCmd(token string) *cobra.Command {
	var userType string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List CRM users",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().StringVar(&userType, "type", "", "user category, e.g. AllUsers|ActiveUsers|CurrentUser|AdminUsers")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		path := "/users"
		if strings.TrimSpace(userType) != "" {
			q := url.Values{}
			q.Set("type", strings.TrimSpace(userType))
			path += "?" + q.Encode()
		}
		body, err := s.call(cmd.Context(), token, http.MethodGet, path, nil)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newUserMeCmd is `user me` — sugar for `user list --type CurrentUser`, the
// currently authenticated CRM user (also the identity probe used by the
// provider bundle).
func (s *Service) newUserMeCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "me",
		Short: "Show the currently authenticated CRM user",
		Args:  cobra.NoArgs,
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		body, err := s.call(cmd.Context(), token, http.MethodGet, "/users?type=CurrentUser", nil)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newOrgGetCmd is top-level `org get` — GET /crm/v8/org: the organization
// record.
func (s *Service) newOrgGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "org",
		Short: "Show the organization record",
		Args:  cobra.NoArgs,
	}
	get := &cobra.Command{
		Use:   "get",
		Short: "Get the organization record",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/org", nil)
			if err != nil {
				return err
			}
			return s.emitJSON(body)
		},
	}
	cmd.AddCommand(get)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error { return cmd.Help() }
	return cmd
}
