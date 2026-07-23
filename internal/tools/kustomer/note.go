package kustomer

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newNoteListCmd: GET /conversations/{id}/notes (read internal notes).
func (s *Service) newNoteListCmd(base, token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "list <conversation-id>",
		Short:       "List a conversation's internal notes",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
	}
	lf := registerListFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		qs, err := buildQuery(lf.page, lf.pageSize, lf.query)
		if err != nil {
			return err
		}
		body, err := s.call(cmd.Context(), base, token, http.MethodGet, "/conversations/"+url.PathEscape(args[0])+"/notes"+qs, nil)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newNoteCreateCmd: POST /conversations/{id}/notes (leave an internal note).
func (s *Service) newNoteCreateCmd(base, token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "create <conversation-id>",
		Short:       "Add an internal note to a conversation from a JSON body",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
	}
	data, file := registerBodyFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		payload, err := readBody(*data, *file)
		if err != nil {
			return err
		}
		body, err := s.call(cmd.Context(), base, token, http.MethodPost, "/conversations/"+url.PathEscape(args[0])+"/notes", payload)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}
