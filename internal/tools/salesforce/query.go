package salesforce

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

// defaultMaxRecords caps how many records `query` accumulates across
// nextRecordsUrl pages by default. A teammate asking a pipeline question rarely
// needs more; a caller can raise it with --max-records.
const defaultMaxRecords = 2000

// queryPage is one page of a SOQL query result. Records stay raw so the merged
// output is byte-identical to what Salesforce returned per row.
type queryPage struct {
	TotalSize      int               `json:"totalSize"`
	Done           bool              `json:"done"`
	Records        []json.RawMessage `json:"records"`
	NextRecordsURL string            `json:"nextRecordsUrl"`
}

func (s *Service) newQueryCmd(c *client) *cobra.Command {
	var all bool
	var maxRecords int
	cmd := &cobra.Command{
		Use:   "query <soql>",
		Short: "Run a SOQL query (follows pagination up to --max-records)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resource := "/query"
			if all {
				resource = "/queryAll"
			}
			path := dataPath(apiVersion(cmd), resource) + "/?q=" + url.QueryEscape(args[0])
			body, _, err := c.get(cmd.Context(), path)
			if err != nil {
				return err
			}
			var first queryPage
			if err := json.Unmarshal(body, &first); err != nil {
				return &apiError{msg: fmt.Sprintf("salesforce: decode query response: %v", err), err: err}
			}
			records := first.Records
			next := first.NextRecordsURL
			for !first.Done && next != "" && len(records) < maxRecords {
				pageBody, _, pErr := c.get(cmd.Context(), next)
				if pErr != nil {
					return pErr
				}
				var page queryPage
				if err := json.Unmarshal(pageBody, &page); err != nil {
					return &apiError{msg: fmt.Sprintf("salesforce: decode query page: %v", err), err: err}
				}
				records = append(records, page.Records...)
				first.Done = page.Done
				next = page.NextRecordsURL
			}
			if len(records) > maxRecords {
				records = records[:maxRecords]
			}
			merged, err := json.Marshal(map[string]any{
				"totalSize": first.TotalSize,
				"done":      first.Done || next == "",
				"records":   records,
			})
			if err != nil {
				return &apiError{msg: fmt.Sprintf("salesforce: encode query result: %v", err), err: err}
			}
			return s.emit(merged)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "use queryAll (includes deleted/archived records)")
	cmd.Flags().IntVar(&maxRecords, "max-records", defaultMaxRecords, "maximum records to accumulate across pages")
	return cmd
}

// searchRequest is the POST parameterizedSearch body — unambiguous compared to
// the GET query-param form. sobjects/fields/overallLimit are all optional.
type searchRequest struct {
	Q            string          `json:"q"`
	SObjects     []searchSObject `json:"sobjects,omitempty"`
	Fields       []string        `json:"fields,omitempty"`
	OverallLimit int             `json:"overallLimit,omitempty"`
}

type searchSObject struct {
	Name string `json:"name"`
}

func (s *Service) newSearchCmd(c *client) *cobra.Command {
	var objects []string
	var fields []string
	var limit int
	cmd := &cobra.Command{
		Use:   "search <term>",
		Short: "Cross-object search (SOSL via parameterizedSearch)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reqBody := searchRequest{Q: args[0], Fields: fields, OverallLimit: limit}
			for _, name := range objects {
				reqBody.SObjects = append(reqBody.SObjects, searchSObject{Name: name})
			}
			payload, err := json.Marshal(reqBody)
			if err != nil {
				return &apiError{msg: fmt.Sprintf("salesforce: encode search request: %v", err), err: err}
			}
			path := dataPath(apiVersion(cmd), "/parameterizedSearch")
			body, _, callErr := c.call(cmd.Context(), "POST", path, payload)
			if callErr != nil {
				return callErr
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringSliceVar(&objects, "objects", nil, "restrict to these sObjects (e.g. Account,Contact)")
	cmd.Flags().StringSliceVar(&fields, "fields", nil, "default fields returned per object")
	cmd.Flags().IntVar(&limit, "limit", 0, "overall result limit (0 = provider default)")
	return cmd
}

func (s *Service) newWhoamiCmd(c *client) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the connected user and org (OpenID userinfo)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, _, err := c.get(cmd.Context(), "/services/oauth2/userinfo")
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newLimitsCmd(c *client) *cobra.Command {
	return &cobra.Command{
		Use:   "limits",
		Short: "Show org API limits (DailyApiRequests, etc.)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, _, err := c.get(cmd.Context(), dataPath(apiVersion(cmd), "/limits"))
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}
