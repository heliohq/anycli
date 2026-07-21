package figma

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

type fileReadOptions struct {
	fileKey    string
	version    string
	ids        string
	depth      int
	geometry   string
	pluginData string
}

func (s *Service) newFilesCommand(token string) *cobra.Command {
	files := &cobra.Command{Use: "files", Short: "Read Figma files and nodes"}
	files.AddCommand(
		s.newFileGetCommand(token),
		s.newFileMetaCommand(token),
		s.newFileNodesCommand(token),
		s.newOperationCommand(token, operationCommandSpec{Use: "versions", Short: "List file version history", OperationID: "getFileVersions"}),
	)
	return files
}

func (s *Service) newFileGetCommand(token string) *cobra.Command {
	var opts fileReadOptions
	var branchData bool
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a Figma file document",
		Args:  cobra.NoArgs,
		Annotations: map[string]string{
			operationIDAnnotation: "getFile",
			sideEffectAnnotation:  "false", // GET file document
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			query, err := opts.query()
			if err != nil {
				return err
			}
			if branchData {
				query.Set("branch_data", "true")
			}
			params := appendOperationQuery([]string{"file_key=" + opts.fileKey}, query)
			return s.callCatalogOperationAndEmit(cmd.Context(), token, "getFile", params, nil)
		},
	}
	bindFileReadFlags(cmd, &opts, false)
	cmd.Flags().BoolVar(&branchData, "branch-data", false, "include branch metadata")
	return cmd
}

func (s *Service) newFileMetaCommand(token string) *cobra.Command {
	var fileKey string
	cmd := &cobra.Command{
		Use:   "meta",
		Short: "Get lightweight Figma file metadata",
		Args:  cobra.NoArgs,
		Annotations: map[string]string{
			operationIDAnnotation: "getFileMeta",
			sideEffectAnnotation:  "false", // GET file metadata
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.callCatalogOperationAndEmit(cmd.Context(), token, "getFileMeta", []string{"file_key=" + fileKey}, nil)
		},
	}
	cmd.Flags().StringVar(&fileKey, "file-key", "", "Figma file or branch key")
	_ = cmd.MarkFlagRequired("file-key")
	return cmd
}

func (s *Service) newFileNodesCommand(token string) *cobra.Command {
	var opts fileReadOptions
	cmd := &cobra.Command{
		Use:   "nodes",
		Short: "Get specific nodes from a Figma file",
		Args:  cobra.NoArgs,
		Annotations: map[string]string{
			operationIDAnnotation: "getFileNodes",
			sideEffectAnnotation:  "false", // GET file nodes
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			query, err := opts.query()
			if err != nil {
				return err
			}
			params := appendOperationQuery([]string{"file_key=" + opts.fileKey}, query)
			return s.callCatalogOperationAndEmit(cmd.Context(), token, "getFileNodes", params, nil)
		},
	}
	bindFileReadFlags(cmd, &opts, true)
	return cmd
}

func bindFileReadFlags(cmd *cobra.Command, opts *fileReadOptions, requireIDs bool) {
	cmd.Flags().StringVar(&opts.fileKey, "file-key", "", "Figma file or branch key")
	cmd.Flags().StringVar(&opts.version, "version", "", "specific file version ID")
	cmd.Flags().StringVar(&opts.ids, "ids", "", "comma-separated node IDs")
	cmd.Flags().IntVar(&opts.depth, "depth", 0, "positive document traversal depth")
	cmd.Flags().StringVar(&opts.geometry, "geometry", "", "set to paths to include vector geometry")
	cmd.Flags().StringVar(&opts.pluginData, "plugin-data", "", "comma-separated plugin IDs or shared")
	_ = cmd.MarkFlagRequired("file-key")
	if requireIDs {
		_ = cmd.MarkFlagRequired("ids")
	}
}

func (o fileReadOptions) query() (url.Values, error) {
	query := url.Values{}
	setOptionalString(query, "version", o.version)
	setOptionalString(query, "ids", o.ids)
	setOptionalString(query, "geometry", o.geometry)
	setOptionalString(query, "plugin_data", o.pluginData)
	if o.geometry != "" && o.geometry != "paths" {
		return nil, fmt.Errorf("--geometry must be paths")
	}
	if err := setOptionalPositiveInt(query, "depth", o.depth); err != nil {
		return nil, err
	}
	return query, nil
}

func (s *Service) newImagesCommand(token string) *cobra.Command {
	images := &cobra.Command{Use: "images", Short: "Render nodes and read original image fills"}
	images.AddCommand(
		s.newImageRenderCommand(token),
		s.newOperationCommand(token, operationCommandSpec{Use: "fills", Short: "Get original image-fill download URLs", OperationID: "getImageFills"}),
	)
	return images
}

func (s *Service) newImageRenderCommand(token string) *cobra.Command {
	var fileKey, ids, version, format string
	var scale float64
	var svgOutlineText, svgIncludeID, svgIncludeNodeID, svgSimplifyStroke bool
	var contentsOnly, useAbsoluteBounds bool
	cmd := &cobra.Command{
		Use:   "render",
		Short: "Render file nodes to temporary image URLs",
		Args:  cobra.NoArgs,
		Annotations: map[string]string{
			operationIDAnnotation: "getImages",
			sideEffectAnnotation:  "false", // GET rendered image URLs
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if scale != 0 && (scale < 0.01 || scale > 4) {
				return fmt.Errorf("--scale must be between 0.01 and 4")
			}
			if err := validateImageFormat(format); err != nil {
				return err
			}
			query := url.Values{"ids": []string{ids}}
			setOptionalString(query, "version", version)
			setOptionalString(query, "format", format)
			if scale != 0 {
				query.Set("scale", strconv.FormatFloat(scale, 'f', -1, 64))
			}
			setChangedBool(query, cmd, "svg-outline-text", "svg_outline_text", svgOutlineText)
			setChangedBool(query, cmd, "svg-include-id", "svg_include_id", svgIncludeID)
			setChangedBool(query, cmd, "svg-include-node-id", "svg_include_node_id", svgIncludeNodeID)
			setChangedBool(query, cmd, "svg-simplify-stroke", "svg_simplify_stroke", svgSimplifyStroke)
			setChangedBool(query, cmd, "contents-only", "contents_only", contentsOnly)
			setChangedBool(query, cmd, "use-absolute-bounds", "use_absolute_bounds", useAbsoluteBounds)
			params := appendOperationQuery([]string{"file_key=" + fileKey}, query)
			return s.callCatalogOperationAndEmit(cmd.Context(), token, "getImages", params, nil)
		},
	}
	cmd.Flags().StringVar(&fileKey, "file-key", "", "Figma file or branch key")
	cmd.Flags().StringVar(&ids, "ids", "", "comma-separated node IDs to render")
	cmd.Flags().StringVar(&version, "version", "", "specific file version ID")
	cmd.Flags().StringVar(&format, "format", "", "image format: jpg, png, svg, or pdf")
	cmd.Flags().Float64Var(&scale, "scale", 0, "image scale between 0.01 and 4")
	cmd.Flags().BoolVar(&svgOutlineText, "svg-outline-text", false, "render SVG text as outlines")
	cmd.Flags().BoolVar(&svgIncludeID, "svg-include-id", false, "include layer names as SVG IDs")
	cmd.Flags().BoolVar(&svgIncludeNodeID, "svg-include-node-id", false, "include node IDs as SVG attributes")
	cmd.Flags().BoolVar(&svgSimplifyStroke, "svg-simplify-stroke", false, "simplify SVG strokes when possible")
	cmd.Flags().BoolVar(&contentsOnly, "contents-only", false, "exclude overlapping content outside node bounds")
	cmd.Flags().BoolVar(&useAbsoluteBounds, "use-absolute-bounds", false, "render using full node dimensions")
	_ = cmd.MarkFlagRequired("file-key")
	_ = cmd.MarkFlagRequired("ids")
	return cmd
}

func validateImageFormat(format string) error {
	if format != "" && format != "jpg" && format != "png" && format != "svg" && format != "pdf" {
		return fmt.Errorf("--format must be one of jpg, png, svg, pdf")
	}
	return nil
}

func setChangedBool(query url.Values, cmd *cobra.Command, flagName, queryName string, value bool) {
	if cmd.Flags().Changed(flagName) {
		query.Set(queryName, strconv.FormatBool(value))
	}
}
