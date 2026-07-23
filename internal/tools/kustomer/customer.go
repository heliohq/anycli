package kustomer

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newCustomerGetCmd: GET /customers/{id}.
func (s *Service) newCustomerGetCmd(base, token string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <id>",
		Short:       "Get a customer by id",
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
		Use:         "get-by-email <email>",
		Short:       "Get a customer by email address",
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
		Use:         "conversations <id>",
		Short:       "List a customer's conversations",
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
		Use:         "create",
		Short:       "Create a customer from a JSON body",
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
