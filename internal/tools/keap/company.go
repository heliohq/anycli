package keap

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newCompanyCmd(token string) *cobra.Command {
	cmd := newGroupCmd("company", "Companies (list, get, create, update)")
	cmd.AddCommand(
		s.newCompanyListCmd(token),
		s.newCompanyGetCmd(token),
		s.newCompanyCreateCmd(token),
		s.newCompanyUpdateCmd(token),
	)
	return cmd
}

func (s *Service) newCompanyListCmd(token string) *cobra.Command {
	var lf *listFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List companies (GET /v2/companies)",
		Long: "Companies carry their own ids, unrelated to contact ids. This resource has\n" +
			"no delete verb — list, get, create and update are the whole surface, so\n" +
			"removing a company has to happen in Keap itself.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/companies", lf.values(), nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	lf = registerListFlags(cmd)
	return cmd
}

func (s *Service) newCompanyGetCmd(token string) *cobra.Command {
	var fields string
	cmd := &cobra.Command{
		Use:   "get <company-id>",
		Short: "Get a company (GET /v2/companies/{id})",
		Long: "Takes the numeric company id; `company list --filter company_name==Acme`\n" +
			"resolves a name to one. `--fields` trims the response to a comma-separated\n" +
			"projection.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/companies/"+url.PathEscape(args[0]), fieldsQuery(fields), nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&fields, "fields", "", "comma-separated fields to include")
	return cmd
}

func (s *Service) newCompanyCreateCmd(token string) *cobra.Command {
	var name, website, jsonBody string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a company (POST /v2/companies)",
		Long: "`--company-name` is required and is enforced locally before the request —\n" +
			"supplying `company_name` inside `--json-body` also satisfies it. `--website`\n" +
			"is the only other flag; addresses, phone numbers and custom fields go\n" +
			"through `--json-body`, whose keys overlay and win over the flags.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{}
			if name != "" {
				body["company_name"] = name
			}
			if website != "" {
				body["website"] = website
			}
			if err := applyJSONBody(body, jsonBody); err != nil {
				return err
			}
			if _, ok := body["company_name"]; !ok {
				return &usageError{msg: "company create requires --company-name (or company_name in --json-body)"}
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/v2/companies", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&name, "company-name", "", "company name (required)")
	cmd.Flags().StringVar(&website, "website", "", "company website")
	cmd.Flags().StringVar(&jsonBody, "json-body", "", "raw JSON body merged over the flag-built payload")
	return cmd
}

func (s *Service) newCompanyUpdateCmd(token string) *cobra.Command {
	var name, website, jsonBody string
	cmd := &cobra.Command{
		Use:   "update <company-id>",
		Short: "Update a company (PATCH /v2/companies/{id})",
		Long: "Only the fields supplied are touched, and unlike create `--company-name` is\n" +
			"optional here — but at least one of `--company-name`, `--website` or\n" +
			"`--json-body` must carry a value.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{}
			if name != "" {
				body["company_name"] = name
			}
			if website != "" {
				body["website"] = website
			}
			if err := applyJSONBody(body, jsonBody); err != nil {
				return err
			}
			if err := requireBody(body); err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPatch, "/v2/companies/"+url.PathEscape(args[0]), nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&name, "company-name", "", "company name")
	cmd.Flags().StringVar(&website, "website", "", "company website")
	cmd.Flags().StringVar(&jsonBody, "json-body", "", "raw JSON body merged over the flag-built payload")
	return cmd
}
