package figma

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
)

//go:embed operations.json
var embeddedOperationCatalog []byte

var operationPathParameter = regexp.MustCompile(`\{([^{}]+)\}`)

type operationCatalog struct {
	Source     string      `json:"source"`
	Operations []operation `json:"operations"`
}

type operation struct {
	ID                  string   `json:"id"`
	Method              string   `json:"method"`
	Path                string   `json:"path"`
	Summary             string   `json:"summary"`
	PAT                 bool     `json:"pat"`
	Scopes              []string `json:"scopes"`
	PathParams          []string `json:"path_params"`
	QueryParams         []string `json:"query_params"`
	RequiredQueryParams []string `json:"required_query_params"`
	BodyRequired        bool     `json:"body_required"`
}

var (
	catalogOnce sync.Once
	catalogData operationCatalog
	catalogErr  error
)

func loadOperationCatalog() (*operationCatalog, error) {
	catalogOnce.Do(func() {
		catalogErr = json.Unmarshal(embeddedOperationCatalog, &catalogData)
		if catalogErr != nil {
			catalogErr = fmt.Errorf("decode embedded Figma operation catalog: %w", catalogErr)
			return
		}
		if catalogData.Source == "" || len(catalogData.Operations) == 0 {
			catalogErr = fmt.Errorf("embedded Figma operation catalog is empty")
		}
	})
	return &catalogData, catalogErr
}

func (c *operationCatalog) find(id string) (operation, bool) {
	for _, candidate := range c.Operations {
		if candidate.ID == id {
			return candidate, true
		}
	}
	return operation{}, false
}

func (o operation) validate() error {
	if o.ID == "" || o.Summary == "" {
		return fmt.Errorf("ID and summary are required")
	}
	if !allowedAPIMethod(o.Method) {
		return fmt.Errorf("unsupported method %q", o.Method)
	}
	if err := validateAPIPath(o.Path); err != nil {
		return err
	}
	wantPathParams := map[string]struct{}{}
	for _, match := range operationPathParameter.FindAllStringSubmatch(o.Path, -1) {
		wantPathParams[match[1]] = struct{}{}
	}
	gotPathParams, err := uniqueNames(o.PathParams)
	if err != nil {
		return fmt.Errorf("path parameters: %w", err)
	}
	if !sameNames(wantPathParams, gotPathParams) {
		return fmt.Errorf("path placeholders and path_params differ")
	}
	queryParams, err := uniqueNames(o.QueryParams)
	if err != nil {
		return fmt.Errorf("query parameters: %w", err)
	}
	requiredQueryParams, err := uniqueNames(o.RequiredQueryParams)
	if err != nil {
		return fmt.Errorf("required query parameters: %w", err)
	}
	for name := range queryParams {
		if _, exists := gotPathParams[name]; exists {
			return fmt.Errorf("parameter %q is both a path and query parameter", name)
		}
	}
	for name := range requiredQueryParams {
		if _, exists := queryParams[name]; !exists {
			return fmt.Errorf("required query parameter %q is not a query parameter", name)
		}
	}
	return nil
}

func (o operation) resolve(rawParams []string) (string, url.Values, error) {
	pathValues := make(map[string]string, len(o.PathParams))
	query := url.Values{}
	pathNames, _ := uniqueNames(o.PathParams)
	queryNames, _ := uniqueNames(o.QueryParams)
	for _, raw := range rawParams {
		name, value, found := strings.Cut(raw, "=")
		if !found || name == "" {
			return "", nil, fmt.Errorf("--param must use key=value form")
		}
		if _, isPath := pathNames[name]; isPath {
			if value == "" {
				return "", nil, fmt.Errorf("path parameter %s must not be empty", name)
			}
			if _, exists := pathValues[name]; exists {
				return "", nil, fmt.Errorf("path parameter %s may be set only once", name)
			}
			pathValues[name] = value
			continue
		}
		if _, isQuery := queryNames[name]; isQuery {
			query.Add(name, value)
			continue
		}
		return "", nil, fmt.Errorf("unknown parameter %s for operation %s", name, o.ID)
	}

	path := o.Path
	for _, name := range o.PathParams {
		value, exists := pathValues[name]
		if !exists {
			return "", nil, fmt.Errorf("missing required parameter %s for operation %s", name, o.ID)
		}
		path = strings.ReplaceAll(path, "{"+name+"}", url.PathEscape(value))
	}
	for _, name := range o.RequiredQueryParams {
		values, exists := query[name]
		if !exists || !containsNonEmpty(values) {
			return "", nil, fmt.Errorf("missing required query parameter %s for operation %s", name, o.ID)
		}
	}
	return path, query, nil
}

func containsNonEmpty(values []string) bool {
	for _, value := range values {
		if value != "" {
			return true
		}
	}
	return false
}

func uniqueNames(names []string) (map[string]struct{}, error) {
	result := make(map[string]struct{}, len(names))
	for _, name := range names {
		if name == "" {
			return nil, fmt.Errorf("parameter name must not be empty")
		}
		if _, exists := result[name]; exists {
			return nil, fmt.Errorf("duplicate parameter %q", name)
		}
		result[name] = struct{}{}
	}
	return result, nil
}

func sameNames(left, right map[string]struct{}) bool {
	if len(left) != len(right) {
		return false
	}
	for name := range left {
		if _, exists := right[name]; !exists {
			return false
		}
	}
	return true
}
