package figma

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"

	"github.com/heliohq/anycli/internal/tools/figma/contextdata"
	"github.com/spf13/cobra"
)

const (
	defaultContextDepth    = 4
	defaultContextMaxNodes = 1000
)

type contextSource struct {
	FileKey  string `json:"file_key"`
	NodeIDs  string `json:"node_ids"`
	FileType string `json:"file_type,omitempty"`
}

type designContextOutput struct {
	Source    contextSource   `json:"source"`
	Nodes     json.RawMessage `json:"nodes"`
	Renders   json.RawMessage `json:"renders"`
	Variables json.RawMessage `json:"variables,omitempty"`
}

type catalogBatchRequest struct {
	Name        string
	OperationID string
	Params      []string
}

type catalogBatchResult struct {
	Body json.RawMessage
	Err  error
}

func (s *Service) newContextCommand(token string) *cobra.Command {
	group := &cobra.Command{Use: "context", Short: "Build bounded, agent-oriented Figma context"}
	group.AddCommand(
		s.newContextMetadataCommand(token),
		s.newDesignContextCommand(token, "design", "Get design nodes, renders, and optional variables"),
		s.newDesignContextCommand(token, "figjam", "Get FigJam nodes, renders, and optional variables"),
		s.newContextScreenshotCommand(token),
		s.newContextVariablesCommand(token),
	)
	return group
}

func (s *Service) newContextMetadataCommand(token string) *cobra.Command {
	var locatorFlags locatorOptions
	var depth, maxNodes int
	cmd := &cobra.Command{
		Use:   "metadata",
		Short: "Get a bounded sparse tree of node metadata",
		Args:  cobra.NoArgs,
		// GET getFile / getFileNodes; local extraction only.
		Annotations: map[string]string{sideEffectAnnotation: "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			locator, err := locatorFlags.resolve()
			if err != nil {
				return err
			}
			if depth <= 0 {
				return fmt.Errorf("--depth must be positive")
			}
			if maxNodes <= 0 {
				return fmt.Errorf("--max-nodes must be positive")
			}
			operationID := "getFile"
			params := []string{"file_key=" + locator.FileKey, "depth=" + strconv.Itoa(depth)}
			if locator.NodeIDs != "" {
				operationID = "getFileNodes"
				params = append(params, "ids="+locator.NodeIDs)
			}
			body, err := s.callCatalogOperation(cmd.Context(), token, operationID, params, nil)
			if err != nil {
				return err
			}
			metadata, err := contextdata.ExtractMetadata(body, maxNodes)
			if err != nil {
				return err
			}
			encoded, err := json.Marshal(metadata)
			if err != nil {
				return fmt.Errorf("encode Figma metadata: %w", err)
			}
			return s.emit(encoded)
		},
	}
	bindLocatorFlags(cmd, &locatorFlags)
	cmd.Flags().IntVar(&depth, "depth", defaultContextDepth, "positive Figma document traversal depth")
	cmd.Flags().IntVar(&maxNodes, "max-nodes", defaultContextMaxNodes, "maximum nodes in the normalized output")
	return cmd
}

func (s *Service) newDesignContextCommand(token, use, short string) *cobra.Command {
	var locatorFlags locatorOptions
	var depth int
	var includeGeometry, includeVariables bool
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		// GET getFileNodes + getImages (+ getLocalVariables) batch.
		Annotations: map[string]string{sideEffectAnnotation: "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			locator, err := locatorFlags.resolve()
			if err != nil {
				return err
			}
			if err := validateContextFileType(locator, use); err != nil {
				return err
			}
			if locator.NodeIDs == "" {
				return fmt.Errorf("context %s requires a node-id in --url or --ids", use)
			}
			if depth <= 0 {
				return fmt.Errorf("--depth must be positive")
			}
			if use == "figjam" && includeVariables {
				return fmt.Errorf("--include-variables is available only for Figma Design files")
			}
			nodeParams := []string{"file_key=" + locator.FileKey, "ids=" + locator.NodeIDs, "depth=" + strconv.Itoa(depth)}
			if includeGeometry {
				nodeParams = append(nodeParams, "geometry=paths")
			}
			requests := []catalogBatchRequest{
				{Name: "nodes", OperationID: "getFileNodes", Params: nodeParams},
				{Name: "renders", OperationID: "getImages", Params: []string{"file_key=" + locator.FileKey, "ids=" + locator.NodeIDs, "format=png", "scale=1"}},
			}
			if includeVariables {
				requests = append(requests, catalogBatchRequest{Name: "variables", OperationID: "getLocalVariables", Params: []string{"file_key=" + locator.FileKey}})
			}
			responses, err := s.callCatalogBatch(cmd.Context(), token, requests)
			if err != nil {
				return err
			}
			output := designContextOutput{
				Source:    contextSource(locator),
				Nodes:     responses["nodes"],
				Renders:   responses["renders"],
				Variables: responses["variables"],
			}
			encoded, err := json.Marshal(output)
			if err != nil {
				return fmt.Errorf("encode Figma design context: %w", err)
			}
			return s.emit(encoded)
		},
	}
	bindLocatorFlags(cmd, &locatorFlags)
	cmd.Flags().IntVar(&depth, "depth", defaultContextDepth, "positive Figma document traversal depth")
	cmd.Flags().BoolVar(&includeGeometry, "include-geometry", false, "include vector path geometry")
	cmd.Flags().BoolVar(&includeVariables, "include-variables", false, "include local variables (Enterprise)")
	return cmd
}

func (s *Service) newContextScreenshotCommand(token string) *cobra.Command {
	var locatorFlags locatorOptions
	var format string
	var scale float64
	cmd := &cobra.Command{
		Use:   "screenshot",
		Short: "Get temporary rendered image URLs for selected nodes",
		Args:  cobra.NoArgs,
		Annotations: map[string]string{
			operationIDAnnotation: "getImages",
			sideEffectAnnotation:  "false", // GET rendered image URLs
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			locator, err := locatorFlags.resolve()
			if err != nil {
				return err
			}
			if locator.NodeIDs == "" {
				return fmt.Errorf("context screenshot requires a node-id in --url or --ids")
			}
			if scale < 0.01 || scale > 4 {
				return fmt.Errorf("--scale must be between 0.01 and 4")
			}
			if err := validateImageFormat(format); err != nil {
				return err
			}
			params := []string{"file_key=" + locator.FileKey, "ids=" + locator.NodeIDs, "format=" + format, "scale=" + strconv.FormatFloat(scale, 'f', -1, 64)}
			return s.callCatalogOperationAndEmit(cmd.Context(), token, "getImages", params, nil)
		},
	}
	bindLocatorFlags(cmd, &locatorFlags)
	cmd.Flags().StringVar(&format, "format", "png", "image format: jpg, png, svg, or pdf")
	cmd.Flags().Float64Var(&scale, "scale", 1, "image scale between 0.01 and 4")
	return cmd
}

func (s *Service) newContextVariablesCommand(token string) *cobra.Command {
	var locatorFlags locatorOptions
	cmd := &cobra.Command{
		Use:   "variables",
		Short: "Get local variables for a file (Enterprise)",
		Args:  cobra.NoArgs,
		Annotations: map[string]string{
			operationIDAnnotation: "getLocalVariables",
			sideEffectAnnotation:  "false", // GET local variables
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			locator, err := locatorFlags.resolve()
			if err != nil {
				return err
			}
			if locator.FileType == "board" {
				return fmt.Errorf("context variables is available only for Figma Design files")
			}
			return s.callCatalogOperationAndEmit(cmd.Context(), token, "getLocalVariables", []string{"file_key=" + locator.FileKey}, nil)
		},
	}
	bindLocatorFlags(cmd, &locatorFlags)
	return cmd
}

func validateContextFileType(locator figmaLocator, contextType string) error {
	if locator.FileType == "" {
		return nil
	}
	if contextType == "figjam" && locator.FileType != "board" {
		return fmt.Errorf("context figjam requires a FigJam board URL or an explicit --file-key")
	}
	if contextType == "design" && locator.FileType == "board" {
		return fmt.Errorf("context design does not accept a FigJam board URL; use context figjam")
	}
	return nil
}

func (s *Service) callCatalogBatch(ctx context.Context, token string, requests []catalogBatchRequest) (map[string]json.RawMessage, error) {
	results := make([]catalogBatchResult, len(requests))
	var wait sync.WaitGroup
	wait.Add(len(requests))
	for index, request := range requests {
		go func() {
			defer wait.Done()
			body, err := s.callCatalogOperation(ctx, token, request.OperationID, request.Params, nil)
			if err == nil && !json.Valid(body) {
				err = fmt.Errorf("response is not valid JSON")
			}
			results[index] = catalogBatchResult{Body: body, Err: err}
		}()
	}
	wait.Wait()
	responses := make(map[string]json.RawMessage, len(requests))
	for index, request := range requests {
		if results[index].Err != nil {
			return nil, fmt.Errorf("figma context %s: %w", request.Name, results[index].Err)
		}
		responses[request.Name] = results[index].Body
	}
	return responses, nil
}
