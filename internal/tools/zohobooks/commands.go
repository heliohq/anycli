package zohobooks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// stringFlag pairs a resource-specific CLI flag with the Books query param it
// maps to. Values are passed through verbatim, so status views like
// Status.Overdue and _contains/_startswith field variants work without a
// client-side allowlist.
type stringFlag struct{ name, query, usage string }

// requireOrg validates the persistent --organization-id flag on an org-scoped
// command. A missing value is a usage error (exit 2) that points the agent at
// `org list` — there is no "pick a default org" fallback.
func requireOrg(orgID string) (string, error) {
	if strings.TrimSpace(orgID) == "" {
		return "", &usageError{msg: "--organization-id is required; run `zoho-books org list` to discover your organization ids"}
	}
	return strings.TrimSpace(orgID), nil
}

// newListCmd builds `<resource> list` — GET /books/v3/{path}?organization_id=…
// with the shared pagination flags (--page, --per-page) plus any
// resource-specific filters. The provider JSON envelope (incl. page_context)
// is printed verbatim.
func (s *Service) newListCmd(token string, orgID *string, path string, extra []stringFlag) *cobra.Command {
	var page, perPage int
	vals := make(map[string]*string, len(extra))
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List " + path,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
	}
	for _, f := range extra {
		p := new(string)
		vals[f.query] = p
		cmd.Flags().StringVar(p, f.name, "", f.usage)
	}
	cmd.Flags().IntVar(&page, "page", 0, "1-based page number")
	cmd.Flags().IntVar(&perPage, "per-page", 0, "records per page (default/max 200)")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		org, err := requireOrg(*orgID)
		if err != nil {
			return err
		}
		q := url.Values{}
		q.Set("organization_id", org)
		for key, p := range vals {
			if strings.TrimSpace(*p) != "" {
				q.Set(key, *p)
			}
		}
		if page > 0 {
			q.Set("page", strconv.Itoa(page))
		}
		if perPage > 0 {
			q.Set("per_page", strconv.Itoa(perPage))
		}
		body, err := s.call(cmd.Context(), token, http.MethodGet, "/"+path+"?"+q.Encode(), nil)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newGetCmd builds `<resource> get` — GET /books/v3/{path}/{id}?organization_id=…
func (s *Service) newGetCmd(token string, orgID *string, path string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get one " + strings.TrimSuffix(path, "s") + " by id",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
	}
	cmd.Flags().StringVar(&id, "id", "", "record id (required)")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		org, err := requireOrg(*orgID)
		if err != nil {
			return err
		}
		if strings.TrimSpace(id) == "" {
			return &usageError{msg: "--id is required"}
		}
		q := url.Values{}
		q.Set("organization_id", org)
		p := "/" + path + "/" + url.PathEscape(strings.TrimSpace(id)) + "?" + q.Encode()
		body, err := s.call(cmd.Context(), token, http.MethodGet, p, nil)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newCreateCmd builds `<resource> create` — POST /books/v3/{path}?organization_id=…
// with --data sent as the raw request body. Books create endpoints take a flat
// JSON object (line-item-bearing creates put line_items inside --data), NOT a
// {"data":[…]} wrapper — this is the Books/CRM divergence.
func (s *Service) newCreateCmd(token string, orgID *string, path string) *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create one " + strings.TrimSuffix(path, "s"),
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"},
	}
	cmd.Flags().StringVar(&data, "data", "", "JSON object for the new record (required)")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		org, err := requireOrg(*orgID)
		if err != nil {
			return err
		}
		raw, err := rawObject(data)
		if err != nil {
			return err
		}
		q := url.Values{}
		q.Set("organization_id", org)
		body, err := s.call(cmd.Context(), token, http.MethodPost, "/"+path+"?"+q.Encode(), raw)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newOrgCmd builds the top-level `org` group and its `list` verb —
// GET /books/v3/organizations, the one endpoint that takes NO organization_id
// and yields the ids every other command requires.
func (s *Service) newOrgCmd(token string) *cobra.Command {
	org := newGroupCmd("org", "Discover the organizations this login can operate in")
	list := &cobra.Command{
		Use:         "list",
		Short:       "List organizations (no --organization-id; discovers the ids)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/organizations", nil)
			if err != nil {
				return err
			}
			return s.emitJSON(body)
		},
	}
	org.AddCommand(list)
	return org
}

// rawObject validates that --data is a non-empty single JSON object and returns
// the original bytes for verbatim passthrough. Empty is a usage error; invalid
// JSON is a usage error; a non-object JSON value is rejected.
func rawObject(val string) ([]byte, error) {
	trimmed := strings.TrimSpace(val)
	if trimmed == "" {
		return nil, &usageError{msg: "--data is required (a single JSON object)"}
	}
	var probe any
	if err := json.Unmarshal([]byte(trimmed), &probe); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("--data is not valid JSON: %v", err)}
	}
	if _, ok := probe.(map[string]any); !ok {
		return nil, &usageError{msg: "--data must be a single JSON object"}
	}
	return []byte(trimmed), nil
}
