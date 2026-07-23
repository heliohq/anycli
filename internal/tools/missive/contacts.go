package missive

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newContactsCmd builds the `contacts` group: list/search, read, and
// create/update contacts within a contact book (CRM sync).
func (s *Service) newContactsCmd(token string) *cobra.Command {
	group := newGroupCmd("contacts", "List, read, and sync contacts within a contact book")
	group.AddCommand(
		s.newContactsListCmd(token),
		s.newContactsGetCmd(token),
		s.newContactsCreateCmd(token),
		s.newContactsUpdateCmd(token),
	)
	return group
}

func (s *Service) newContactsListCmd(token string) *cobra.Command {
	var (
		contactBook    string
		limit, offset  int
		search         string
		modifiedSince  string
		order          string
		includeDeleted bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List/search contacts in a contact book",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("contact_book", contactBook)
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			if offset > 0 {
				q.Set("offset", strconv.Itoa(offset))
			}
			setStr(q, "search", search)
			setStr(q, "modified_since", modifiedSince)
			setStr(q, "order", order)
			setBoolFilter(q, "include_deleted", includeDeleted)

			body, err := s.call(cmd.Context(), token, http.MethodGet, "/contacts", q, nil)
			if err != nil {
				return err
			}
			return s.emitOffsetList(body, "contacts", offset, limit)
		},
	}
	f := cmd.Flags()
	f.StringVar(&contactBook, "contact-book", "", "contact book id (required)")
	f.IntVar(&limit, "limit", 50, "max contacts (Missive max 200)")
	f.IntVar(&offset, "offset", 0, "pagination offset")
	f.StringVar(&search, "search", "", "full-text search filter")
	f.StringVar(&modifiedSince, "modified-since", "", "only contacts modified since this Unix timestamp")
	f.StringVar(&order, "order", "", "sort order")
	f.BoolVar(&includeDeleted, "include-deleted", false, "include deleted contacts")
	_ = cmd.MarkFlagRequired("contact-book")
	return cmd
}

func (s *Service) newContactsGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <contact-id>",
		Short: "Show one contact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/contacts/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newContactsCreateCmd(token string) *cobra.Command {
	var inline, file string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create contacts (POST /contacts). Body: {\"contacts\":[{...}]}",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := s.decodeJSONBody(inline, file, cmd.InOrStdin())
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/contacts", nil, payload)
			if err != nil {
				return err
			}
			return s.emitBodyOrOK(body)
		},
	}
	addBodyFlags(cmd, &inline, &file)
	return cmd
}

func (s *Service) newContactsUpdateCmd(token string) *cobra.Command {
	var inline, file string
	cmd := &cobra.Command{
		Use:   "update <contact-id[,contact-id...]>",
		Short: "Update contacts (PATCH /contacts/:ids). Body: {\"contacts\":[{...}]}",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := s.decodeJSONBody(inline, file, cmd.InOrStdin())
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodPatch, "/contacts/"+url.PathEscape(args[0]), nil, payload)
			if err != nil {
				return err
			}
			return s.emitBodyOrOK(body)
		},
	}
	addBodyFlags(cmd, &inline, &file)
	return cmd
}
