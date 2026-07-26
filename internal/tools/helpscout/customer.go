package helpscout

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newCustomerCmd(token string) *cobra.Command {
	cmd := newGroupCmd("customer", "Look up and maintain customer records")
	cmd.AddCommand(
		s.newCustomerListCmd(token),
		s.newCustomerGetCmd(token),
		s.newCustomerCreateCmd(token),
		s.newCustomerUpdateCmd(token),
	)
	return cmd
}

// newCustomerListCmd — GET /customers.
func (s *Service) newCustomerListCmd(token string) *cobra.Command {
	var firstName, lastName, mailbox, modifiedSince, query string
	var page int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List/search customers (GET /customers)",
		Long: "--first-name and --last-name are exact-field filters; --query takes Help\n" +
			"Scout's search string verbatim and is how to find somebody by address\n" +
			"(`email:person@example.com`). --mailbox restricts to customers seen in one\n" +
			"inbox and --modified-since to those touched after an ISO-8601 timestamp.\n" +
			"Page with --page. The ids returned here are what --customer-id takes on\n" +
			"`conversation create` and `thread reply`.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setIf(q, "firstName", firstName)
			setIf(q, "lastName", lastName)
			setIf(q, "mailbox", mailbox)
			setIf(q, "modifiedSince", modifiedSince)
			setIf(q, "query", query)
			setPage(q, page)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/customers", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp.body)
		},
	}
	cmd.Flags().StringVar(&firstName, "first-name", "", "first-name filter")
	cmd.Flags().StringVar(&lastName, "last-name", "", "last-name filter")
	cmd.Flags().StringVar(&mailbox, "mailbox", "", "inbox id filter")
	cmd.Flags().StringVar(&modifiedSince, "modified-since", "", "ISO 8601 timestamp; customers modified after")
	cmd.Flags().StringVar(&query, "query", "", "advanced search string (passed through verbatim)")
	cmd.Flags().IntVar(&page, "page", 0, "1-based page number")
	return cmd
}

// newCustomerGetCmd — GET /customers/{id}.
func (s *Service) newCustomerGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get one customer (GET /customers/{id})",
		Long: "Takes the numeric customer id, not an email address — resolve one with\n" +
			"`customer list --query \"email:person@example.com\"`. Customers and staff\n" +
			"users are separate directories with separate id spaces, so a `user list`\n" +
			"id does not resolve here.",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/customers/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp.body)
		},
	}
	return cmd
}

// newCustomerCreateCmd — POST /customers. 201 → a "created" receipt with the
// new customer id.
func (s *Service) newCustomerCreateCmd(token string) *cobra.Command {
	var firstName, lastName, email, organization, jobTitle string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a customer (POST /customers)",
		Long: "At least one field is required. --email is stored as a single work-type\n" +
			"address; additional addresses, phones and social handles cannot be set\n" +
			"here and `customer update` does not reach them either. The response is an\n" +
			"id/status receipt, and that id is what --customer-id takes elsewhere.\n" +
			"Creating a customer opens no conversation — `conversation create` does\n" +
			"that and can take the email directly.",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{}
			setBodyIf(body, "firstName", firstName)
			setBodyIf(body, "lastName", lastName)
			setBodyIf(body, "organization", organization)
			setBodyIf(body, "jobTitle", jobTitle)
			if email != "" {
				body["emails"] = []any{map[string]any{"type": "work", "value": email}}
			}
			if len(body) == 0 {
				return &usageError{msg: "pass at least one field, e.g. --first-name / --last-name / --email"}
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/customers", nil, body)
			if err != nil {
				return err
			}
			return s.emitReceipt(resp.resourceID(), "created")
		},
	}
	cmd.Flags().StringVar(&firstName, "first-name", "", "first name")
	cmd.Flags().StringVar(&lastName, "last-name", "", "last name")
	cmd.Flags().StringVar(&email, "email", "", "work email")
	cmd.Flags().StringVar(&organization, "organization", "", "organization")
	cmd.Flags().StringVar(&jobTitle, "job-title", "", "job title")
	return cmd
}

// newCustomerUpdateCmd — PATCH /customers/{id}, the API's partial Update
// Customer endpoint. Only the fields passed are changed; omitted fields are
// preserved. This is deliberately NOT PUT /customers/{id} (Overwrite Customer),
// which nulls every field not present in the request — a natural partial
// invocation there silently wipes firstName/lastName/organization/etc. Flags
// compile to a JSON-Patch array of replace ops (the shape Help Scout's Update
// Customer requires); 204 → an "updated" receipt.
func (s *Service) newCustomerUpdateCmd(token string) *cobra.Command {
	var firstName, lastName, organization, jobTitle string
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Partially update a customer's core fields (PATCH /customers/{id})",
		Long: "A PARTIAL update: only --first-name, --last-name, --organization and\n" +
			"--job-title are reachable, each compiled into a JSON-Patch replace op, and\n" +
			"an omitted flag leaves its field untouched. Emails, phones and custom\n" +
			"fields are not editable from here. An empty value counts as unset rather\n" +
			"than as a clear, so no field can be blanked with this command. The answer\n" +
			"is an \"updated\" receipt with no body.",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ops := []map[string]any{}
			addReplaceOp(&ops, "/firstName", firstName)
			addReplaceOp(&ops, "/lastName", lastName)
			addReplaceOp(&ops, "/organization", organization)
			addReplaceOp(&ops, "/jobTitle", jobTitle)
			if len(ops) == 0 {
				return &usageError{msg: "nothing to update: pass at least one field"}
			}
			// Update Customer takes the whole JSON-Patch array in one PATCH; only
			// the listed paths change, so unset flags never overwrite existing
			// fields.
			if _, err := s.call(cmd.Context(), token, http.MethodPatch, "/customers/"+url.PathEscape(args[0]), nil, ops); err != nil {
				return err
			}
			return s.emitReceipt(args[0], "updated")
		},
	}
	cmd.Flags().StringVar(&firstName, "first-name", "", "first name")
	cmd.Flags().StringVar(&lastName, "last-name", "", "last name")
	cmd.Flags().StringVar(&organization, "organization", "", "organization")
	cmd.Flags().StringVar(&jobTitle, "job-title", "", "job title")
	return cmd
}

// setBodyIf writes key=value into body only when value is non-empty.
func setBodyIf(body map[string]any, key, value string) {
	if value != "" {
		body[key] = value
	}
}

// addReplaceOp appends a JSON-Patch replace op for path only when value is
// non-empty, so an unset flag never overwrites the existing customer field.
func addReplaceOp(ops *[]map[string]any, path, value string) {
	if value != "" {
		*ops = append(*ops, map[string]any{"op": "replace", "path": path, "value": value})
	}
}
