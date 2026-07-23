package billcom

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newResourceGroup builds a resource command group (list/get[/create]) over the
// given v3 collection path. allowCreate is false for read-only resources such as
// payments (money-movement carve-out).
func (s *Service) newResourceGroup(c *client, name, path string, allowCreate bool) *cobra.Command {
	g := newGroupCmd(name, "Manage "+name+"s")
	g.AddCommand(s.newListCmd(c, name, path), s.newGetCmd(c, name, path))
	if allowCreate {
		g.AddCommand(s.newCreateCmd(c, name, path))
	}
	return g
}

// newListCmd builds `<resource> list`, exposing BILL's cursor pagination
// (--max / --page) plus passthrough --filter / --sort, and normalizing BILL's
// {results,nextPage} into a provider-neutral {items,next_page} envelope.
func (s *Service) newListCmd(c *client, name, path string) *cobra.Command {
	var (
		max     int
		page    string
		filters []string
		sort    string
	)
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List " + name + "s",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if max > 0 {
				q.Set("max", strconv.Itoa(max))
			}
			if page != "" {
				q.Set("page", page)
			}
			for _, f := range filters {
				q.Add("filters", f)
			}
			if sort != "" {
				q.Set("sort", sort)
			}
			body, err := c.do(cmd.Context(), http.MethodGet, path, q, nil)
			if err != nil {
				return err
			}
			return s.emitList(body)
		},
	}
	cmd.Flags().IntVar(&max, "max", 0, "maximum number of results to return")
	cmd.Flags().StringVar(&page, "page", "", "pagination token (the next_page value from a prior call)")
	cmd.Flags().StringArrayVar(&filters, "filter", nil, "BILL filter, e.g. field:op:value (repeatable)")
	cmd.Flags().StringVar(&sort, "sort", "", "BILL sort expression, e.g. field:asc")
	return cmd
}

// newGetCmd builds `<resource> get <id>`.
func (s *Service) newGetCmd(c *client, name, path string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <id>",
		Short:       "Get one " + name + " by id",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := c.do(cmd.Context(), http.MethodGet, path+"/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emitRaw(body)
		},
	}
}

// newCreateCmd builds `<resource> create --data <json>`, posting the supplied
// JSON object as the request body.
func (s *Service) newCreateCmd(c *client, name, path string) *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a " + name + " from a JSON body (--data)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if data == "" {
				return &usageError{msg: "create requires --data with a JSON object body"}
			}
			var payload any
			if err := json.Unmarshal([]byte(data), &payload); err != nil {
				return &usageError{msg: fmt.Sprintf("invalid --data JSON: %v", err)}
			}
			body, err := c.do(cmd.Context(), http.MethodPost, path, nil, payload)
			if err != nil {
				return err
			}
			return s.emitRaw(body)
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "JSON object body for the new "+name)
	return cmd
}

// newOrgCmd builds `org list` — the login organizations for these credentials.
func (s *Service) newOrgCmd(c *client) *cobra.Command {
	g := newGroupCmd("org", "List login organizations")
	g.AddCommand(&cobra.Command{
		Use:         "list",
		Short:       "List organizations available to this login",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := c.do(cmd.Context(), http.MethodGet, "/organizations", nil, nil)
			if err != nil {
				return err
			}
			return s.emitRaw(body)
		},
	})
	return g
}

// newWhoamiCmd builds `whoami` — getsessioninfo (org id, user id, MFA status).
func (s *Service) newWhoamiCmd(c *client) *cobra.Command {
	return &cobra.Command{
		Use:         "whoami",
		Short:       "Show the current BILL session info (org id, user id, MFA status)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := c.do(cmd.Context(), http.MethodGet, "/login/session", nil, nil)
			if err != nil {
				return err
			}
			return s.emitRaw(body)
		},
	}
}

// emitRaw writes a provider JSON response to stdout verbatim.
func (s *Service) emitRaw(body []byte) error {
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// emitList normalizes BILL's {results,nextPage} list response into a
// provider-neutral {items,next_page} envelope.
func (s *Service) emitList(body []byte) error {
	var in struct {
		Results  json.RawMessage `json:"results"`
		NextPage string          `json:"nextPage"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		return &apiError{msg: fmt.Sprintf("bill-com: decode list response: %v", err), err: err}
	}
	items := in.Results
	if len(items) == 0 {
		items = json.RawMessage("[]")
	}
	out, err := json.Marshal(struct {
		Items    json.RawMessage `json:"items"`
		NextPage string          `json:"next_page"`
	}{Items: items, NextPage: in.NextPage})
	if err != nil {
		return &apiError{msg: fmt.Sprintf("bill-com: encode list envelope: %v", err), err: err}
	}
	return s.emitRaw(out)
}
