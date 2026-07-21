package figma

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

const operationIDAnnotation = "figma-operation-id"

// sideEffectAnnotation is the anycli side-effect fact key (design 318): "true"
// when the command may issue a mutating provider API call under any input.
const sideEffectAnnotation = "anycli.side_effect"

type operationCommandSpec struct {
	Use         string
	Short       string
	OperationID string
}

// operationSideEffect derives the anycli.side_effect fact from the pinned
// catalog's HTTP method: GET never mutates; every other method may.
func operationSideEffect(method string) string {
	if method == http.MethodGet {
		return "false"
	}
	return "true"
}

func (s *Service) newOperationCommand(token string, spec operationCommandSpec) *cobra.Command {
	operation, err := findOperation(spec.OperationID)
	if err != nil {
		return &cobra.Command{
			Use:   spec.Use,
			Short: spec.Short,
			Args:  cobra.NoArgs,
			// The intended operation is unknown when the catalog lookup
			// fails, so take the safe-side default explicitly.
			Annotations: map[string]string{sideEffectAnnotation: "true"},
			RunE: func(*cobra.Command, []string) error {
				return fmt.Errorf("build Figma command %s: %w", spec.Use, err)
			},
		}
	}
	parameterNames := append([]string{}, operation.PathParams...)
	parameterNames = append(parameterNames, operation.QueryParams...)
	parameterValues := make([]string, len(parameterNames))
	var bodyOptions jsonBodyOptions

	cmd := &cobra.Command{
		Use:   spec.Use,
		Short: spec.Short,
		Args:  cobra.NoArgs,
		Annotations: map[string]string{
			operationIDAnnotation: operation.ID,
			sideEffectAnnotation:  operationSideEffect(operation.Method),
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			params := make([]string, 0, len(parameterNames))
			for index, name := range parameterNames {
				if parameterValues[index] != "" {
					params = append(params, name+"="+parameterValues[index])
				}
			}
			payload, err := bodyOptions.payload()
			if err != nil {
				return err
			}
			return s.callCatalogOperationAndEmit(cmd.Context(), token, operation.ID, params, payload)
		},
	}
	for index, name := range parameterNames {
		cmd.Flags().StringVar(&parameterValues[index], operationFlagName(name), "", "Figma API parameter "+name)
	}
	for _, name := range operation.PathParams {
		_ = cmd.MarkFlagRequired(operationFlagName(name))
	}
	for _, name := range operation.RequiredQueryParams {
		_ = cmd.MarkFlagRequired(operationFlagName(name))
	}
	if operation.BodyRequired {
		bindJSONBodyFlags(cmd, &bodyOptions)
	}
	return cmd
}

func (s *Service) callCatalogOperation(ctx context.Context, token, operationID string, params []string, payload any) ([]byte, error) {
	operation, path, query, err := resolveCatalogRequest(operationID, params, payload)
	if err != nil {
		return nil, err
	}
	return s.call(ctx, token, operation.Method, path, query, payload)
}

func (s *Service) callCatalogOperationAndEmit(ctx context.Context, token, operationID string, params []string, payload any) error {
	operation, path, query, err := resolveCatalogRequest(operationID, params, payload)
	if err != nil {
		return err
	}
	return s.callAndEmit(ctx, token, operation.Method, path, query, payload)
}

func resolveCatalogRequest(operationID string, params []string, payload any) (operation, string, url.Values, error) {
	operation, err := findOperation(operationID)
	if err != nil {
		return operation, "", nil, err
	}
	if !operation.PAT {
		return operation, "", nil, fmt.Errorf("figma operation %s does not accept a personal access token", operation.ID)
	}
	path, query, err := operation.resolve(params)
	if err != nil {
		return operation, "", nil, err
	}
	if operation.BodyRequired && payload == nil {
		return operation, "", nil, fmt.Errorf("figma operation %s requires --body-json or --body-file", operation.ID)
	}
	if !operation.BodyRequired && payload != nil {
		return operation, "", nil, fmt.Errorf("figma operation %s does not accept a request body", operation.ID)
	}
	return operation, path, query, nil
}

func operationFlagName(parameter string) string {
	switch parameter {
	case "maxwidth":
		return "max-width"
	case "maxheight":
		return "max-height"
	default:
		return strings.ReplaceAll(parameter, "_", "-")
	}
}

func appendOperationQuery(params []string, query url.Values) []string {
	for name, values := range query {
		for _, value := range values {
			params = append(params, name+"="+value)
		}
	}
	return params
}
