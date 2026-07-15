package figma

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

const maxRequestBodyBytes = 4 << 20

type apiOptions struct {
	method  string
	path    string
	queries []string
	body    jsonBodyOptions
}

type jsonBodyOptions struct {
	bodyJSON string
	bodyFile string
}

func (s *Service) newAPICommand(token string) *cobra.Command {
	var opts apiOptions
	cmd := &cobra.Command{
		Use:   "api",
		Short: "Call any PAT-accessible Figma REST endpoint",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			method, query, payload, err := opts.request()
			if err != nil {
				return err
			}
			return s.callAndEmit(cmd.Context(), token, method, opts.path, query, payload)
		},
	}
	cmd.Flags().StringVar(&opts.method, "method", http.MethodGet, "HTTP method: GET, POST, PUT, PATCH, or DELETE")
	cmd.Flags().StringVar(&opts.path, "path", "", "Figma API path beginning with /v1/ or /v2/")
	cmd.Flags().StringArrayVar(&opts.queries, "query", nil, "query parameter in key=value form (repeatable)")
	bindJSONBodyFlags(cmd, &opts.body)
	_ = cmd.MarkFlagRequired("path")
	cmd.AddCommand(
		s.newAPIListCommand(),
		s.newAPIDescribeCommand(),
		s.newAPICallCommand(token),
	)
	return cmd
}

func (o apiOptions) request() (string, url.Values, any, error) {
	method := strings.ToUpper(o.method)
	if !allowedAPIMethod(method) {
		return "", nil, nil, fmt.Errorf("--method must be one of GET, POST, PUT, PATCH, DELETE")
	}
	if err := validateAPIPath(o.path); err != nil {
		return "", nil, nil, err
	}
	query, err := parseAPIQuery(o.queries)
	if err != nil {
		return "", nil, nil, err
	}
	payload, err := o.body.payload()
	if err != nil {
		return "", nil, nil, err
	}
	return method, query, payload, nil
}

func allowedAPIMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func validateAPIPath(path string) error {
	if !strings.HasPrefix(path, "/v1/") && !strings.HasPrefix(path, "/v2/") {
		return fmt.Errorf("--path must start with /v1/ or /v2/")
	}
	if strings.ContainsAny(path, "?#") {
		return fmt.Errorf("--path must not contain a query or fragment; use --query")
	}
	decoded, err := url.PathUnescape(path)
	if err != nil {
		return fmt.Errorf("--path contains invalid escaping: %w", err)
	}
	for _, segment := range strings.Split(decoded, "/") {
		if segment == ".." {
			return fmt.Errorf("--path must not contain path traversal")
		}
	}
	if strings.Contains(decoded, "\\") {
		return fmt.Errorf("--path must not contain path traversal")
	}
	return nil
}

func parseAPIQuery(values []string) (url.Values, error) {
	query := url.Values{}
	for _, value := range values {
		key, item, found := strings.Cut(value, "=")
		if !found || key == "" {
			return nil, fmt.Errorf("--query must use key=value form")
		}
		query.Add(key, item)
	}
	return query, nil
}

func bindJSONBodyFlags(cmd *cobra.Command, opts *jsonBodyOptions) {
	cmd.Flags().StringVar(&opts.bodyJSON, "body-json", "", "inline JSON request body")
	cmd.Flags().StringVar(&opts.bodyFile, "body-file", "", "path to a JSON request body (maximum 4 MiB)")
}

func (o jsonBodyOptions) payload() (any, error) {
	if o.bodyJSON == "" && o.bodyFile == "" {
		return nil, nil
	}
	if o.bodyJSON != "" && o.bodyFile != "" {
		return nil, fmt.Errorf("--body-json and --body-file are mutually exclusive")
	}
	raw := []byte(o.bodyJSON)
	flag := "--body-json"
	if o.bodyFile != "" {
		var err error
		raw, err = readBoundedFile(o.bodyFile, maxRequestBodyBytes)
		if err != nil {
			return nil, fmt.Errorf("--body-file: %w", err)
		}
		flag = "--body-file"
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("%s must be valid JSON", flag)
	}
	return json.RawMessage(raw), nil
}

func (s *Service) newAPIListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List operations from the pinned official Figma OpenAPI catalog",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			catalog, err := loadOperationCatalog()
			if err != nil {
				return err
			}
			body, err := json.Marshal(catalog)
			if err != nil {
				return fmt.Errorf("encode Figma operation catalog: %w", err)
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newAPIDescribeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "describe OPERATION_ID",
		Short: "Describe one Figma REST operation",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			operation, err := findOperation(args[0])
			if err != nil {
				return err
			}
			body, err := json.Marshal(operation)
			if err != nil {
				return fmt.Errorf("encode Figma operation: %w", err)
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newAPICallCommand(token string) *cobra.Command {
	var params []string
	var bodyOptions jsonBodyOptions
	cmd := &cobra.Command{
		Use:   "call OPERATION_ID",
		Short: "Call a catalogued Figma REST operation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := bodyOptions.payload()
			if err != nil {
				return err
			}
			return s.callCatalogOperationAndEmit(cmd.Context(), token, args[0], params, payload)
		},
	}
	cmd.Flags().StringArrayVar(&params, "param", nil, "path or query parameter in key=value form (repeatable)")
	bindJSONBodyFlags(cmd, &bodyOptions)
	return cmd
}

func findOperation(id string) (operation, error) {
	catalog, err := loadOperationCatalog()
	if err != nil {
		return operation{}, err
	}
	result, ok := catalog.find(id)
	if !ok {
		return operation{}, fmt.Errorf("unknown Figma operation %q; run `figma api list`", id)
	}
	return result, nil
}

func readBoundedFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("request body exceeds %d bytes", limit)
	}
	return raw, nil
}
