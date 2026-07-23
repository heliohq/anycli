package keap

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// Notes are contact-scoped in v2: /v2/contacts/{contact_id}/notes[/{note_id}].
func (s *Service) newNoteCmd(token string) *cobra.Command {
	cmd := newGroupCmd("note", "Contact notes (list, get, create, update, delete)")
	cmd.AddCommand(
		s.newNoteListCmd(token),
		s.newNoteGetCmd(token),
		s.newNoteCreateCmd(token),
		s.newNoteUpdateCmd(token),
		s.newNoteDeleteCmd(token),
	)
	return cmd
}

func notesPath(contactID string) string {
	return "/v2/contacts/" + url.PathEscape(contactID) + "/notes"
}

func notePath(contactID, noteID string) string {
	return notesPath(contactID) + "/" + url.PathEscape(noteID)
}

func (s *Service) newNoteListCmd(token string) *cobra.Command {
	var contactID string
	var lf *listFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List a contact's notes (GET /v2/contacts/{id}/notes)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if contactID == "" {
				return &usageError{msg: "--contact-id is required"}
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, notesPath(contactID), lf.values(), nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&contactID, "contact-id", "", "contact id (required)")
	lf = registerListFlags(cmd)
	return cmd
}

func (s *Service) newNoteGetCmd(token string) *cobra.Command {
	var contactID, noteID string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a contact note (GET /v2/contacts/{id}/notes/{note_id})",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if contactID == "" || noteID == "" {
				return &usageError{msg: "--contact-id and --note-id are required"}
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, notePath(contactID, noteID), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&contactID, "contact-id", "", "contact id (required)")
	cmd.Flags().StringVar(&noteID, "note-id", "", "note id (required)")
	return cmd
}

func (s *Service) newNoteCreateCmd(token string) *cobra.Command {
	var contactID, userID, text, title, noteType, jsonBody string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a contact note (POST /v2/contacts/{id}/notes)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if contactID == "" {
				return &usageError{msg: "--contact-id is required"}
			}
			body := map[string]any{}
			if userID != "" {
				body["user_id"] = userID
			}
			if text != "" {
				body["text"] = text
			}
			if title != "" {
				body["title"] = title
			}
			if noteType != "" {
				body["type"] = noteType
			}
			if err := applyJSONBody(body, jsonBody); err != nil {
				return err
			}
			if _, ok := body["user_id"]; !ok {
				return &usageError{msg: "note create requires --user-id (or user_id in --json-body)"}
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, notesPath(contactID), nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&contactID, "contact-id", "", "contact id (required)")
	cmd.Flags().StringVar(&userID, "user-id", "", "authoring user id (required)")
	cmd.Flags().StringVar(&text, "text", "", "note body text")
	cmd.Flags().StringVar(&title, "title", "", "note title")
	cmd.Flags().StringVar(&noteType, "type", "", "note type")
	cmd.Flags().StringVar(&jsonBody, "json-body", "", "raw JSON body merged over the flag-built payload")
	return cmd
}

func (s *Service) newNoteUpdateCmd(token string) *cobra.Command {
	var contactID, noteID, text, title, noteType, jsonBody string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a contact note (PATCH /v2/contacts/{id}/notes/{note_id})",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if contactID == "" || noteID == "" {
				return &usageError{msg: "--contact-id and --note-id are required"}
			}
			body := map[string]any{}
			if text != "" {
				body["text"] = text
			}
			if title != "" {
				body["title"] = title
			}
			if noteType != "" {
				body["type"] = noteType
			}
			if err := applyJSONBody(body, jsonBody); err != nil {
				return err
			}
			if err := requireBody(body); err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPatch, notePath(contactID, noteID), nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&contactID, "contact-id", "", "contact id (required)")
	cmd.Flags().StringVar(&noteID, "note-id", "", "note id (required)")
	cmd.Flags().StringVar(&text, "text", "", "note body text")
	cmd.Flags().StringVar(&title, "title", "", "note title")
	cmd.Flags().StringVar(&noteType, "type", "", "note type")
	cmd.Flags().StringVar(&jsonBody, "json-body", "", "raw JSON body merged over the flag-built payload")
	return cmd
}

func (s *Service) newNoteDeleteCmd(token string) *cobra.Command {
	var contactID, noteID string
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a contact note (DELETE /v2/contacts/{id}/notes/{note_id})",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if contactID == "" || noteID == "" {
				return &usageError{msg: "--contact-id and --note-id are required"}
			}
			resp, err := s.call(cmd.Context(), token, http.MethodDelete, notePath(contactID, noteID), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&contactID, "contact-id", "", "contact id (required)")
	cmd.Flags().StringVar(&noteID, "note-id", "", "note id (required)")
	return cmd
}
