package kustomer

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newCustomerGetCmd: GET /customers/{id}.
func (s *Service) newCustomerGetCmd(base, token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get a customer by id",
		Long: "Takes Kustomer's internal customer id. An email address will not resolve\n" +
			"here — that is `customer get-by-email`. The record carries the person's\n" +
			"emails, phones and custom attributes, but not their tickets: those are\n" +
			"`customer conversations <id>`.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), base, token, http.MethodGet, "/customers/"+url.PathEscape(args[0]), nil)
			if err != nil {
				return err
			}
			return s.emitJSON(body)
		},
	}
}

// newCustomerGetByEmailCmd: GET /customers/email={email}. The lookup value is a
// URL-encoded segment of the path (the externalId= / phone= variants share this
// exact form).
func (s *Service) newCustomerGetByEmailCmd(base, token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get-by-email <email>",
		Short: "Get a customer by email address",
		Long: "The usual entry point, since an inbound request almost always arrives with\n" +
			"an address rather than an id. The match is on an email Kustomer has stored\n" +
			"for the customer, so an alias the customer has never written from does not\n" +
			"resolve; `search customers` is the fuzzier fallback. The id in the reply\n" +
			"is what every conversation command then takes.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), base, token, http.MethodGet, "/customers/email="+url.PathEscape(args[0]), nil)
			if err != nil {
				return err
			}
			return s.emitJSON(body)
		},
	}
}

// newCustomerConversationsCmd: GET /customers/{id}/conversations.
func (s *Service) newCustomerConversationsCmd(base, token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "conversations <id>",
		Short: "List a customer's conversations",
		Long: "One customer's whole ticket history, which is the context to read before\n" +
			"answering them — `conversation list` spans the org and will not narrow to\n" +
			"a person. Paged with `--page` / `--page-size`; add status or date\n" +
			"narrowing through repeatable `--query key=value` filters.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
	}
	lf := registerListFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		qs, err := buildQuery(lf.page, lf.pageSize, lf.query)
		if err != nil {
			return err
		}
		body, err := s.call(cmd.Context(), base, token, http.MethodGet, "/customers/"+url.PathEscape(args[0])+"/conversations"+qs, nil)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newCustomerCreateCmd: POST /customers with a raw JSON body.
func (s *Service) newCustomerCreateCmd(base, token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a customer from a JSON body",
		Long: "The body is Kustomer's own customer shape, sent unmodified — typically\n" +
			"`{\"name\":…,\"emails\":[{\"email\":…}]}`, where emails and phones are ARRAYS of\n" +
			"objects rather than plain strings. Creating with an address that already\n" +
			"exists makes a SECOND customer rather than merging, and this tool has no\n" +
			"merge or delete verb, so check `customer get-by-email` first.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
	}
	data, file := registerBodyFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		payload, err := readBody(*data, *file)
		if err != nil {
			return err
		}
		body, err := s.call(cmd.Context(), base, token, http.MethodPost, "/customers", payload)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}
