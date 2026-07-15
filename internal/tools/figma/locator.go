package figma

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

type figmaLocator struct {
	FileKey  string
	NodeIDs  string
	FileType string
}

type locatorOptions struct {
	URL     string
	FileKey string
	NodeIDs string
}

func bindLocatorFlags(cmd *cobra.Command, opts *locatorOptions) {
	cmd.Flags().StringVar(&opts.URL, "url", "", "Figma file or node URL")
	cmd.Flags().StringVar(&opts.FileKey, "file-key", "", "Figma file or branch key")
	cmd.Flags().StringVar(&opts.NodeIDs, "ids", "", "comma-separated node IDs (overrides the URL node-id)")
}

func (o locatorOptions) resolve() (figmaLocator, error) {
	if o.URL != "" && o.FileKey != "" {
		return figmaLocator{}, fmt.Errorf("--url and --file-key are mutually exclusive")
	}
	if o.URL == "" && o.FileKey == "" {
		return figmaLocator{}, fmt.Errorf("one of --url or --file-key is required")
	}
	if o.FileKey != "" {
		return figmaLocator{FileKey: o.FileKey, NodeIDs: o.NodeIDs}, nil
	}
	locator, err := parseFigmaURL(o.URL)
	if err != nil {
		return figmaLocator{}, err
	}
	if o.NodeIDs != "" {
		locator.NodeIDs = o.NodeIDs
	}
	return locator, nil
}

func parseFigmaURL(raw string) (figmaLocator, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return figmaLocator{}, fmt.Errorf("--url must be an absolute HTTPS URL")
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "figma.com" && !strings.HasSuffix(host, ".figma.com") {
		return figmaLocator{}, fmt.Errorf("--url must be on figma.com")
	}
	parts := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	if len(parts) < 2 || !supportedFigmaFileType(parts[0]) {
		return figmaLocator{}, fmt.Errorf("--url must identify a Design or FigJam file supported by PAT REST (design, file, proto, or board)")
	}
	fileKey, err := url.PathUnescape(parts[1])
	if err != nil || fileKey == "" {
		return figmaLocator{}, fmt.Errorf("--url contains an invalid file key")
	}
	nodeID := parsed.Query().Get("node-id")
	if !strings.Contains(nodeID, ":") {
		nodeID = strings.ReplaceAll(nodeID, "-", ":")
	}
	return figmaLocator{FileKey: fileKey, NodeIDs: nodeID, FileType: parts[0]}, nil
}

func supportedFigmaFileType(value string) bool {
	switch value {
	case "design", "file", "proto", "board":
		return true
	default:
		return false
	}
}
