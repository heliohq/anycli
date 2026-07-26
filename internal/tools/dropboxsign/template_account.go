package dropboxsign

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newTemplateListCmd lists the reusable templates the account can send with.
func (s *Service) newTemplateListCmd(token string) *cobra.Command {
	var (
		page     int
		pageSize int
		query    string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List reusable templates",
		Long: "Templates are documents pre-prepared in the Dropbox Sign web app with signer\n" +
			"roles and form fields already placed. This tool sends with them but cannot\n" +
			"create, edit or delete one. The ids returned here are what\n" +
			"`signature-request send-with-template` takes via `--template`.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if page > 0 {
				q.Set("page", strconv.Itoa(page))
			}
			if pageSize > 0 {
				q.Set("page_size", strconv.Itoa(pageSize))
			}
			if query != "" {
				q.Set("query", query)
			}
			body, err := s.callGET(cmd.Context(), token, "/template/list", q)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().IntVar(&page, "page", 0, "page number (1-based)")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "results per page")
	cmd.Flags().StringVar(&query, "query", "", "filter query (Dropbox Sign search syntax)")
	return cmd
}

// newTemplateGetCmd fetches one template's roles and fields.
func (s *Service) newTemplateGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <template_id>",
		Short: "Get one template (roles and fields)",
		Long: "The `signer_roles[]` in the response are exactly the role names\n" +
			"`signature-request send-with-template` demands in its \"Role:Name:email\"\n" +
			"signers; any other role name fails the send. `custom_fields[]` and\n" +
			"`named_form_fields[]` describe what the template will ask each signer to fill\n" +
			"in, which is what makes a template send cheaper than uploading a document and\n" +
			"placing fields by hand.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.callGET(cmd.Context(), token, "/template/"+url.PathEscape(args[0]), nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// newAccountGetCmd returns the authenticated account's identity and quota. It
// is also the provider identity endpoint (the bearer token identifies the
// account, so no query params are needed).
func (s *Service) newAccountGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "Get the authenticated account (identity and quota)",
		Long: "Also the identity call: the bearer credential alone selects the account, so\n" +
			"this takes no arguments and answers \"whose Dropbox Sign am I connected to\".\n" +
			"The response carries the remaining signature and template quota for the\n" +
			"current billing period — worth reading before a batch of real sends, since\n" +
			"exhausting it fails the send, while `--test-mode` requests never draw it down.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.callGET(cmd.Context(), token, "/account", nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}
