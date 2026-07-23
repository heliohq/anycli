package gumroad

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func (s *Service) newLicenseCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "license", Short: "Software license keys (verify, enable, disable)"}
	cmd.AddCommand(
		s.newLicenseVerifyCmd(token),
		s.newLicenseEnableCmd(token),
		s.newLicenseDisableCmd(token),
	)
	return cmd
}

func (s *Service) newLicenseVerifyCmd(token string) *cobra.Command {
	var productID, licenseKey string
	var increment bool
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify a license key (POST /licenses/verify)",
		Args:  cobra.NoArgs,
		// Verification is a read by default. It only mutates (consumes a use)
		// when --increment-uses-count is set, but the annotation must be
		// static; the endpoint can increment, so classify it as a side effect.
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			form := url.Values{}
			form.Set("product_id", productID)
			form.Set("license_key", licenseKey)
			form.Set("increment_uses_count", strconv.FormatBool(increment))
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/licenses/verify", form)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&productID, "product-id", "", "product id the license belongs to")
	cmd.Flags().StringVar(&licenseKey, "license-key", "", "the license key to verify")
	cmd.Flags().BoolVar(&increment, "increment-uses-count", false, "count this verification as a use (default false)")
	_ = cmd.MarkFlagRequired("product-id")
	_ = cmd.MarkFlagRequired("license-key")
	return cmd
}

func (s *Service) newLicenseEnableCmd(token string) *cobra.Command {
	var productID, licenseKey string
	cmd := &cobra.Command{
		Use:         "enable",
		Short:       "Enable a license key (PUT /licenses/enable)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"}, // PUT
		RunE: func(cmd *cobra.Command, _ []string) error {
			form := url.Values{}
			form.Set("product_id", productID)
			form.Set("license_key", licenseKey)
			body, err := s.call(cmd.Context(), token, http.MethodPut, "/licenses/enable", form)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&productID, "product-id", "", "product id the license belongs to")
	cmd.Flags().StringVar(&licenseKey, "license-key", "", "the license key to enable")
	_ = cmd.MarkFlagRequired("product-id")
	_ = cmd.MarkFlagRequired("license-key")
	return cmd
}

func (s *Service) newLicenseDisableCmd(token string) *cobra.Command {
	var productID, licenseKey string
	cmd := &cobra.Command{
		Use:         "disable",
		Short:       "Disable a license key (PUT /licenses/disable)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"}, // PUT
		RunE: func(cmd *cobra.Command, _ []string) error {
			form := url.Values{}
			form.Set("product_id", productID)
			form.Set("license_key", licenseKey)
			body, err := s.call(cmd.Context(), token, http.MethodPut, "/licenses/disable", form)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&productID, "product-id", "", "product id the license belongs to")
	cmd.Flags().StringVar(&licenseKey, "license-key", "", "the license key to disable")
	_ = cmd.MarkFlagRequired("product-id")
	_ = cmd.MarkFlagRequired("license-key")
	return cmd
}
