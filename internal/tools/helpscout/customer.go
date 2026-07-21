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
		Args:  cobra.NoArgs,
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
		Args:  cobra.ExactArgs(1),
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
		Args:  cobra.NoArgs,
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

// newCustomerUpdateCmd — PUT /customers/{id}. Overwrites the customer's core
// fields; 204 → an "updated" receipt.
func (s *Service) newCustomerUpdateCmd(token string) *cobra.Command {
	var firstName, lastName, organization, jobTitle string
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a customer's core fields (PUT /customers/{id})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{}
			setBodyIf(body, "firstName", firstName)
			setBodyIf(body, "lastName", lastName)
			setBodyIf(body, "organization", organization)
			setBodyIf(body, "jobTitle", jobTitle)
			if len(body) == 0 {
				return &usageError{msg: "nothing to update: pass at least one field"}
			}
			if _, err := s.call(cmd.Context(), token, http.MethodPut, "/customers/"+url.PathEscape(args[0]), nil, body); err != nil {
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
