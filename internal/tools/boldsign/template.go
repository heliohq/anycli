package boldsign

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func (s *Service) newTemplateCmd(token string) *cobra.Command {
	cmd := newGroupCmd("template", "Discover and send from reusable templates")
	cmd.AddCommand(
		s.newTemplateListCmd(token),
		s.newTemplateGetCmd(token),
		s.newTemplateSendCmd(token),
	)
	return cmd
}

func (s *Service) newTemplateListCmd(token string) *cobra.Command {
	var search string
	var page, pageSize int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List reusable templates (GET /v1/template/list)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("page", strconv.Itoa(page))
			if pageSize > 0 {
				q.Set("pageSize", strconv.Itoa(pageSize))
			}
			if search != "" {
				q.Set("searchKey", search)
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v1/template/list", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&page, "page", 1, "page number (1-based)")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "templates per page (BoldSign default 10)")
	cmd.Flags().StringVar(&search, "search", "", "search by template title or id")
	return cmd
}

func (s *Service) newTemplateGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a template's properties and roles (GET /v1/template/properties)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("templateId", id)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v1/template/properties", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "template id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newTemplateSendCmd(token string) *cobra.Command {
	var id, title, message, onBehalfOf string
	var roles, fields []string
	var signingOrder bool
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send a document from a template (POST /v1/template/send)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if len(roles) == 0 {
				return &usageError{msg: "boldsign: at least one --role is required"}
			}

			roleList := make([]map[string]any, 0, len(roles))
			for _, spec := range roles {
				index, party, err := parseRoleSpec(spec)
				if err != nil {
					return err
				}
				entry := map[string]any{
					"RoleIndex":   index,
					"SignerName":  party.name,
					"SignerEmail": party.email,
				}
				if signingOrder {
					entry["SignerOrder"] = index
				}
				roleList = append(roleList, entry)
			}

			body := map[string]any{"Roles": roleList}
			if title != "" {
				body["Title"] = title
			}
			if message != "" {
				body["Message"] = message
			}
			if signingOrder {
				body["EnableSigningOrder"] = true
			}
			if onBehalfOf != "" {
				body["OnBehalfOf"] = onBehalfOf
			}
			if len(fields) > 0 {
				existing := make([]map[string]any, 0, len(fields))
				for _, spec := range fields {
					fieldID, value, err := parseFieldSpec(spec)
					if err != nil {
						return err
					}
					existing = append(existing, map[string]any{"Id": fieldID, "Value": value})
				}
				body["ExistingFormFields"] = existing
			}

			q := url.Values{}
			q.Set("templateId", id)
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/v1/template/send", q, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "template id")
	cmd.Flags().StringVar(&title, "title", "", "override the document title")
	cmd.Flags().StringVar(&message, "message", "", "message shown to all recipients")
	cmd.Flags().StringArrayVar(&roles, "role", nil, "role binding as \"<roleIndex>:Name <email>\" (repeatable, at least one)")
	cmd.Flags().StringArrayVar(&fields, "field", nil, "prefill an existing form field as \"<fieldId>=<value>\" (repeatable)")
	cmd.Flags().BoolVar(&signingOrder, "signing-order", false, "enforce sequential signing order (SignerOrder = RoleIndex)")
	cmd.Flags().StringVar(&onBehalfOf, "on-behalf-of", "", "sender email to send on behalf of")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}
