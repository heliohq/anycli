package figma

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

type renderedImagesResponse struct {
	Images map[string]*string `json:"images"`
}

type imageFillsResponse struct {
	Meta struct {
		Images map[string]string `json:"images"`
	} `json:"meta"`
}

func (s *Service) newAssetsCommand(token string) *cobra.Command {
	assets := &cobra.Command{Use: "assets", Short: "Download rendered nodes and original image fills"}
	assets.AddCommand(
		s.newAssetsDownloadCommand(token),
		s.newImageFillsDownloadCommand(token),
	)
	return assets
}

func (s *Service) newAssetsDownloadCommand(token string) *cobra.Command {
	var locatorFlags locatorOptions
	var outputDir, format string
	var scale float64
	var overwrite bool
	cmd := &cobra.Command{
		Use:   "download",
		Short: "Render selected nodes and download the resulting assets",
		Args:  cobra.NoArgs,
		// GET getImages + GET asset URLs; writes local files only.
		Annotations: map[string]string{sideEffectAnnotation: "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			locator, err := locatorFlags.resolve()
			if err != nil {
				return err
			}
			if locator.NodeIDs == "" {
				return fmt.Errorf("assets download requires a node-id in --url or --ids")
			}
			if err := validateImageFormat(format); err != nil {
				return err
			}
			if scale < 0.01 || scale > 4 {
				return fmt.Errorf("--scale must be between 0.01 and 4")
			}
			params := []string{
				"file_key=" + locator.FileKey,
				"ids=" + locator.NodeIDs,
				"format=" + format,
				"scale=" + strconv.FormatFloat(scale, 'f', -1, 64),
			}
			body, err := s.callCatalogOperation(cmd.Context(), token, "getImages", params, nil)
			if err != nil {
				return err
			}
			var response renderedImagesResponse
			if err := json.Unmarshal(body, &response); err != nil {
				return fmt.Errorf("decode Figma rendered images: %w", err)
			}
			sources, err := renderedAssetSources(response.Images, format)
			if err != nil {
				return err
			}
			manifest, err := s.downloadAssets(cmd.Context(), sources, outputDir, overwrite)
			if err != nil {
				return err
			}
			return s.emitJSON(manifest)
		},
	}
	bindLocatorFlags(cmd, &locatorFlags)
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "directory for downloaded assets")
	cmd.Flags().StringVar(&format, "format", "png", "image format: jpg, png, svg, or pdf")
	cmd.Flags().Float64Var(&scale, "scale", 1, "image scale between 0.01 and 4")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "replace existing asset files")
	_ = cmd.MarkFlagRequired("output-dir")
	return cmd
}

func (s *Service) newImageFillsDownloadCommand(token string) *cobra.Command {
	var locatorFlags locatorOptions
	var outputDir string
	var overwrite bool
	cmd := &cobra.Command{
		Use:   "download-fills",
		Short: "Download every original image fill in a file",
		Args:  cobra.NoArgs,
		// GET getImageFills + GET asset URLs; writes local files only.
		Annotations: map[string]string{sideEffectAnnotation: "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			locator, err := locatorFlags.resolve()
			if err != nil {
				return err
			}
			body, err := s.callCatalogOperation(cmd.Context(), token, "getImageFills", []string{"file_key=" + locator.FileKey}, nil)
			if err != nil {
				return err
			}
			var response imageFillsResponse
			if err := json.Unmarshal(body, &response); err != nil {
				return fmt.Errorf("decode Figma image fills: %w", err)
			}
			sources, err := fillAssetSources(response.Meta.Images)
			if err != nil {
				return err
			}
			manifest, err := s.downloadAssets(cmd.Context(), sources, outputDir, overwrite)
			if err != nil {
				return err
			}
			return s.emitJSON(manifest)
		},
	}
	bindLocatorFlags(cmd, &locatorFlags)
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "directory for downloaded image fills")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "replace existing asset files")
	_ = cmd.MarkFlagRequired("output-dir")
	return cmd
}

func renderedAssetSources(images map[string]*string, format string) ([]assetSource, error) {
	if len(images) == 0 {
		return nil, fmt.Errorf("figma returned no rendered images")
	}
	sources := make([]assetSource, 0, len(images))
	for id, rawURL := range images {
		if rawURL == nil || *rawURL == "" {
			return nil, fmt.Errorf("figma could not render node %s", id)
		}
		sources = append(sources, assetSource{ID: id, URL: *rawURL, Extension: "." + format})
	}
	return sources, nil
}

func fillAssetSources(images map[string]string) ([]assetSource, error) {
	if len(images) == 0 {
		return nil, fmt.Errorf("figma returned no image fills")
	}
	sources := make([]assetSource, 0, len(images))
	for reference, rawURL := range images {
		if rawURL == "" {
			return nil, fmt.Errorf("figma returned an empty URL for image fill %s", reference)
		}
		sources = append(sources, assetSource{ID: reference, URL: rawURL})
	}
	return sources, nil
}
