package iterable

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newTemplateCmd groups the template verbs.
func (s *Service) newTemplateCmd(cred credential) *cobra.Command {
	cmd := newGroupCmd("template", "Inspect message templates")
	cmd.AddCommand(s.newTemplateListCmd(cred))
	return cmd
}

func (s *Service) newTemplateListCmd(cred credential) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all templates (GET /api/templates)",
		Long: "Returns the project's message templates with their ids. Templates are\n" +
			"read-only through this tool — there is no create, update or preview\n" +
			"command, and authoring stays in Iterable.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), cred, http.MethodGet, "/api/templates", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

// newEmailCmd groups the transactional-email verbs.
func (s *Service) newEmailCmd(cred credential) *cobra.Command {
	cmd := newGroupCmd("email", "Send transactional email")
	cmd.AddCommand(s.newEmailSendCmd(cred))
	return cmd
}

func (s *Service) newEmailSendCmd(cred credential) *cobra.Command {
	var body string
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send an existing campaign/template email to a user (POST /api/email/target)",
		Long: "This sends a real email to a real address immediately; there is no preview\n" +
			"or test mode. --body is required and pairs an EXISTING campaign with a\n" +
			"recipient (`campaignId` from `campaign list`, plus `recipientEmail`) —\n" +
			"subject and body cannot be supplied here, they come from the campaign's\n" +
			"template. Per-send merge values go under `dataFields`.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := decodeJSONFlag("body", body)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), cred, http.MethodPost, "/api/email/target", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&body, "body", "", `JSON body, e.g. {"campaignId":123,"recipientEmail":"a@b.com"} (required)`)
	_ = cmd.MarkFlagRequired("body")
	return cmd
}

// newCatalogCmd groups the catalog verbs.
func (s *Service) newCatalogCmd(cred credential) *cobra.Command {
	cmd := newGroupCmd("catalog", "Inspect catalogs")
	cmd.AddCommand(s.newCatalogListCmd(cred))
	return cmd
}

func (s *Service) newCatalogListCmd(cred credential) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all catalogs (GET /api/catalogs)",
		Long: "Returns the project's catalogs — the product or content data sets\n" +
			"templates draw recommendations from. Only the catalogs themselves are\n" +
			"listed; there is no command here for reading or writing catalog items.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), cred, http.MethodGet, "/api/catalogs", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}
