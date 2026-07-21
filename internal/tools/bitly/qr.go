package bitly

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

func (s *Service) newQRCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "qr", Short: "QR codes (create, get, list, update, image, scans)"}
	cmd.AddCommand(
		s.newQRCreateCmd(token),
		s.newQRCreateStaticCmd(token),
		s.newQRGetCmd(token),
		s.newQRListCmd(token),
		s.newQRUpdateCmd(token),
		s.newQRImageCmd(token),
		s.newQRScansCmd(token),
	)
	return cmd
}

func (s *Service) newQRCreateCmd(token string) *cobra.Command {
	var group, destBitlink, destLongURL, title, expirationAt, customizationsJSON string
	var tags []string
	var archived bool
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a dynamic QR code (POST /qr-codes)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"}, // POST /qr-codes
		RunE: func(cmd *cobra.Command, _ []string) error {
			if (destBitlink == "") == (destLongURL == "") {
				return fmt.Errorf("bitly: exactly one of --destination-bitlink or --destination-long-url is required")
			}
			guid, err := s.resolveGroup(cmd.Context(), token, group)
			if err != nil {
				return err
			}
			destination := map[string]any{}
			if destBitlink != "" {
				destination["bitlink_id"] = destBitlink
			} else {
				destination["long_url"] = destLongURL
			}
			body := map[string]any{
				"group_guid":  guid,
				"destination": destination,
			}
			if title != "" {
				body["title"] = title
			}
			if len(tags) > 0 {
				body["tags"] = tags
			}
			if expirationAt != "" {
				body["expiration_at"] = expirationAt
			}
			if cmd.Flags().Changed("archived") {
				body["archived"] = archived
			}
			if customizationsJSON != "" {
				v, err := decodeJSONFlag("customizations-json", customizationsJSON)
				if err != nil {
					return err
				}
				body["render_customizations"] = v
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/qr-codes", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&group, "group", "", "group guid (auto-resolved when omitted)")
	cmd.Flags().StringVar(&destBitlink, "destination-bitlink", "", "destination bitlink id (exclusive with --destination-long-url)")
	cmd.Flags().StringVar(&destLongURL, "destination-long-url", "", "destination long URL (exclusive with --destination-bitlink)")
	cmd.Flags().StringVar(&title, "title", "", "QR code title")
	cmd.Flags().StringArrayVar(&tags, "tags", nil, "tag (repeatable)")
	cmd.Flags().StringVar(&expirationAt, "expiration-at", "", "ISO-8601 expiration timestamp")
	cmd.Flags().BoolVar(&archived, "archived", false, "archive state")
	cmd.Flags().StringVar(&customizationsJSON, "customizations-json", "", "render_customizations JSON (raw passthrough)")
	return cmd
}

func (s *Service) newQRCreateStaticCmd(token string) *cobra.Command {
	var content, group, customizationsJSON string
	cmd := &cobra.Command{
		Use:         "create-static",
		Short:       "Create a static QR code (POST /qr-codes/static)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"}, // POST /qr-codes/static
		RunE: func(cmd *cobra.Command, _ []string) error {
			guid, err := s.resolveGroup(cmd.Context(), token, group)
			if err != nil {
				return err
			}
			body := map[string]any{
				"content":    content,
				"group_guid": guid,
			}
			if customizationsJSON != "" {
				v, err := decodeJSONFlag("customizations-json", customizationsJSON)
				if err != nil {
					return err
				}
				body["render_customizations"] = v
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/qr-codes/static", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&content, "content", "", "static QR payload")
	cmd.Flags().StringVar(&group, "group", "", "group guid (auto-resolved when omitted)")
	cmd.Flags().StringVar(&customizationsJSON, "customizations-json", "", "render_customizations JSON (raw passthrough)")
	_ = cmd.MarkFlagRequired("content")
	return cmd
}

func (s *Service) newQRGetCmd(token string) *cobra.Command {
	var qr string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get a QR code (GET /qr-codes/{qrcode_id})",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/qr-codes/"+url.PathEscape(qr), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&qr, "qr", "", "qrcode id")
	_ = cmd.MarkFlagRequired("qr")
	return cmd
}

func (s *Service) newQRListCmd(token string) *cobra.Command {
	var group, searchAfter, query, createdBefore, createdAfter, archived string
	var tags []string
	var size int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List a group's QR codes (GET /groups/{group_guid}/qr-codes)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			guid, err := s.resolveGroup(cmd.Context(), token, group)
			if err != nil {
				return err
			}
			q := url.Values{}
			q.Set("size", intToString(size))
			q.Set("archived", archived)
			if searchAfter != "" {
				q.Set("search_after", searchAfter)
			}
			if query != "" {
				q.Set("query", query)
			}
			if createdBefore != "" {
				q.Set("created_before", createdBefore)
			}
			if createdAfter != "" {
				q.Set("created_after", createdAfter)
			}
			for _, tag := range tags {
				q.Add("tags", tag)
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/groups/"+url.PathEscape(guid)+"/qr-codes", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&group, "group", "", "group guid (auto-resolved when omitted)")
	cmd.Flags().IntVar(&size, "size", 50, "page size")
	cmd.Flags().StringVar(&searchAfter, "search-after", "", "pagination cursor")
	cmd.Flags().StringVar(&query, "query", "", "keyword search")
	cmd.Flags().StringVar(&createdBefore, "created-before", "", "created-before unix timestamp")
	cmd.Flags().StringVar(&createdAfter, "created-after", "", "created-after unix timestamp")
	cmd.Flags().StringVar(&archived, "archived", "off", "archived filter: on|off|both")
	cmd.Flags().StringArrayVar(&tags, "tags", nil, "tag filter (repeatable)")
	return cmd
}

func (s *Service) newQRUpdateCmd(token string) *cobra.Command {
	var qr, title, expirationAt, customizationsJSON string
	var tags []string
	var archived bool
	cmd := &cobra.Command{
		Use:         "update",
		Short:       "Update a QR code (PATCH /qr-codes/{qrcode_id})",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"}, // PATCH /qr-codes/{qrcode_id}
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{}
			if title != "" {
				body["title"] = title
			}
			if len(tags) > 0 {
				body["tags"] = tags
			}
			if cmd.Flags().Changed("archived") {
				body["archived"] = archived
			}
			if expirationAt != "" {
				body["expiration_at"] = expirationAt
			}
			if customizationsJSON != "" {
				v, err := decodeJSONFlag("customizations-json", customizationsJSON)
				if err != nil {
					return err
				}
				body["render_customizations"] = v
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPatch, "/qr-codes/"+url.PathEscape(qr), nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&qr, "qr", "", "qrcode id")
	cmd.Flags().StringVar(&title, "title", "", "QR code title")
	cmd.Flags().StringArrayVar(&tags, "tags", nil, "tag (repeatable)")
	cmd.Flags().BoolVar(&archived, "archived", false, "archive state")
	cmd.Flags().StringVar(&expirationAt, "expiration-at", "", "ISO-8601 expiration timestamp")
	cmd.Flags().StringVar(&customizationsJSON, "customizations-json", "", "render_customizations JSON (raw passthrough)")
	_ = cmd.MarkFlagRequired("qr")
	return cmd
}

// qrImageReceipt is emitted when --output writes the image to a file.
type qrImageReceipt struct {
	QRCodeID string `json:"qrcode_id"`
	Format   string `json:"format"`
	Bytes    int    `json:"bytes"`
	Path     string `json:"path"`
}

// qrImageEnvelope is emitted when no --output is given: base64-encoded image
// data on stdout (never raw binary).
type qrImageEnvelope struct {
	QRCodeID string `json:"qrcode_id"`
	Format   string `json:"format"`
	Encoding string `json:"encoding"`
	Data     string `json:"data"`
}

func (s *Service) newQRImageCmd(token string) *cobra.Command {
	var qr, format, output string
	cmd := &cobra.Command{
		Use:   "image",
		Short: "Fetch a QR code image (GET /qr-codes/{qrcode_id}/image)",
		Args:  cobra.NoArgs,
		// GET only; --output writes a local file, which is not a provider
		// mutation.
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if format != "svg" && format != "png" {
				return fmt.Errorf("bitly: --format must be svg or png, got %q", format)
			}
			data, err := s.fetchQRImage(cmd.Context(), token, qr, format)
			if err != nil {
				return err
			}
			if output != "" {
				if err := os.WriteFile(output, data, 0o644); err != nil {
					return fmt.Errorf("bitly: write image: %w", err)
				}
				return s.emitValue(qrImageReceipt{QRCodeID: qr, Format: format, Bytes: len(data), Path: output})
			}
			return s.emitValue(qrImageEnvelope{
				QRCodeID: qr,
				Format:   format,
				Encoding: "base64",
				Data:     base64.StdEncoding.EncodeToString(data),
			})
		},
	}
	cmd.Flags().StringVar(&qr, "qr", "", "qrcode id")
	cmd.Flags().StringVar(&format, "format", "svg", "image format: svg|png")
	cmd.Flags().StringVar(&output, "output", "", "write raw image bytes to this file")
	_ = cmd.MarkFlagRequired("qr")
	return cmd
}

// fetchQRImage requests the raw image variant. The image endpoint is
// content-negotiated: its JSON variant is the default, so this overrides the
// client's default Accept with the image media type matching format.
func (s *Service) fetchQRImage(ctx context.Context, token, qr, format string) ([]byte, error) {
	accept := "image/svg+xml"
	if format == "png" {
		accept = "image/png"
	}
	q := url.Values{}
	q.Set("format", format)
	requestURL := s.baseURL() + "/qr-codes/" + url.PathEscape(qr) + "/image?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("bitly: build image request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", accept)

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("bitly: GET qr image: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("bitly: read image response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := fmt.Errorf("bitly API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, execution.RejectCredential(apiErr)
		}
		return nil, apiErr
	}
	return body, nil
}
