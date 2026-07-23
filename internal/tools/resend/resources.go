package resend

import (
	"net/http"

	"github.com/spf13/cobra"
)

// --- Audiences ---

func (s *Service) newAudienceCmd(key string) *cobra.Command {
	cmd := newGroupCmd("audience", "Manage audiences (list, get, create, delete)")
	cmd.AddCommand(
		s.newAudienceListCmd(key),
		s.newAudienceGetCmd(key),
		s.newAudienceCreateCmd(key),
		s.newAudienceDeleteCmd(key),
	)
	return cmd
}

func (s *Service) newAudienceListCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:         "list",
		Short:       "List audiences (GET /audiences)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/audiences", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newAudienceGetCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <id>",
		Short:       "Retrieve an audience (GET /audiences/{id})",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/audiences/"+args[0], nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newAudienceCreateCmd(key string) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create an audience (POST /audiences)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), key, http.MethodPost, "/audiences", map[string]any{"name": name}, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "audience name")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func (s *Service) newAudienceDeleteCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:         "delete <id>",
		Short:       "Delete an audience (DELETE /audiences/{id})",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), key, http.MethodDelete, "/audiences/"+args[0], nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

// --- Contacts (nested under an audience) ---

func (s *Service) newContactCmd(key string) *cobra.Command {
	cmd := newGroupCmd("contact", "Manage contacts within an audience (list, get, create, update, delete)")
	cmd.AddCommand(
		s.newContactListCmd(key),
		s.newContactGetCmd(key),
		s.newContactCreateCmd(key),
		s.newContactUpdateCmd(key),
		s.newContactDeleteCmd(key),
	)
	return cmd
}

// audienceFlag registers the required --audience flag shared by contact commands.
func audienceFlag(cmd *cobra.Command, audience *string) {
	cmd.Flags().StringVar(audience, "audience", "", "audience id the contact belongs to")
	_ = cmd.MarkFlagRequired("audience")
}

func (s *Service) newContactListCmd(key string) *cobra.Command {
	var audience string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List contacts in an audience (GET /audiences/{aid}/contacts)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/audiences/"+audience+"/contacts", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	audienceFlag(cmd, &audience)
	return cmd
}

func (s *Service) newContactGetCmd(key string) *cobra.Command {
	var audience string
	cmd := &cobra.Command{
		Use:         "get <id>",
		Short:       "Retrieve a contact by id or email (GET /audiences/{aid}/contacts/{id})",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/audiences/"+audience+"/contacts/"+args[0], nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	audienceFlag(cmd, &audience)
	return cmd
}

func (s *Service) newContactCreateCmd(key string) *cobra.Command {
	var audience, email, firstName, lastName string
	var unsubscribed bool
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a contact (POST /audiences/{aid}/contacts)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{"email": email}
			if firstName != "" {
				body["first_name"] = firstName
			}
			if lastName != "" {
				body["last_name"] = lastName
			}
			if cmd.Flags().Changed("unsubscribed") {
				body["unsubscribed"] = unsubscribed
			}
			resp, err := s.call(cmd.Context(), key, http.MethodPost, "/audiences/"+audience+"/contacts", body, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	audienceFlag(cmd, &audience)
	cmd.Flags().StringVar(&email, "email", "", "contact email")
	cmd.Flags().StringVar(&firstName, "first-name", "", "contact first name")
	cmd.Flags().StringVar(&lastName, "last-name", "", "contact last name")
	cmd.Flags().BoolVar(&unsubscribed, "unsubscribed", false, "unsubscribed state")
	_ = cmd.MarkFlagRequired("email")
	return cmd
}

func (s *Service) newContactUpdateCmd(key string) *cobra.Command {
	var audience, firstName, lastName string
	var unsubscribed bool
	cmd := &cobra.Command{
		Use:         "update <id>",
		Short:       "Update a contact by id or email (PATCH /audiences/{aid}/contacts/{id})",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{}
			if firstName != "" {
				body["first_name"] = firstName
			}
			if lastName != "" {
				body["last_name"] = lastName
			}
			if cmd.Flags().Changed("unsubscribed") {
				body["unsubscribed"] = unsubscribed
			}
			resp, err := s.call(cmd.Context(), key, http.MethodPatch, "/audiences/"+audience+"/contacts/"+args[0], body, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	audienceFlag(cmd, &audience)
	cmd.Flags().StringVar(&firstName, "first-name", "", "contact first name")
	cmd.Flags().StringVar(&lastName, "last-name", "", "contact last name")
	cmd.Flags().BoolVar(&unsubscribed, "unsubscribed", false, "unsubscribed state")
	return cmd
}

func (s *Service) newContactDeleteCmd(key string) *cobra.Command {
	var audience string
	cmd := &cobra.Command{
		Use:         "delete <id>",
		Short:       "Delete a contact by id or email (DELETE /audiences/{aid}/contacts/{id})",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), key, http.MethodDelete, "/audiences/"+audience+"/contacts/"+args[0], nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	audienceFlag(cmd, &audience)
	return cmd
}

// --- Broadcasts ---

func (s *Service) newBroadcastCmd(key string) *cobra.Command {
	cmd := newGroupCmd("broadcast", "Manage broadcasts (list, get, create, update, send, delete)")
	cmd.AddCommand(
		s.newBroadcastListCmd(key),
		s.newBroadcastGetCmd(key),
		s.newBroadcastCreateCmd(key),
		s.newBroadcastUpdateCmd(key),
		s.newBroadcastSendCmd(key),
		s.newBroadcastDeleteCmd(key),
	)
	return cmd
}

func (s *Service) newBroadcastListCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:         "list",
		Short:       "List broadcasts (GET /broadcasts)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/broadcasts", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newBroadcastGetCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <id>",
		Short:       "Retrieve a broadcast (GET /broadcasts/{id})",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/broadcasts/"+args[0], nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newBroadcastCreateCmd(key string) *cobra.Command {
	var audience, from, subject, replyTo, html, text, name string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a broadcast (POST /broadcasts)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{
				"audience_id": audience,
				"from":        from,
				"subject":     subject,
			}
			if replyTo != "" {
				body["reply_to"] = replyTo
			}
			if html != "" {
				body["html"] = html
			}
			if text != "" {
				body["text"] = text
			}
			if name != "" {
				body["name"] = name
			}
			resp, err := s.call(cmd.Context(), key, http.MethodPost, "/broadcasts", body, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&audience, "audience", "", "audience id to send to")
	cmd.Flags().StringVar(&from, "from", "", "sender, `Name <addr>` form; addr must be on a verified domain")
	cmd.Flags().StringVar(&subject, "subject", "", "broadcast subject")
	cmd.Flags().StringVar(&replyTo, "reply-to", "", "reply-to address")
	cmd.Flags().StringVar(&html, "html", "", "HTML body")
	cmd.Flags().StringVar(&text, "text", "", "plain-text body")
	cmd.Flags().StringVar(&name, "name", "", "internal broadcast name")
	_ = cmd.MarkFlagRequired("audience")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("subject")
	return cmd
}

func (s *Service) newBroadcastUpdateCmd(key string) *cobra.Command {
	var subject, replyTo, html, text, name string
	cmd := &cobra.Command{
		Use:         "update <id>",
		Short:       "Update a draft broadcast (PATCH /broadcasts/{id})",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{}
			if subject != "" {
				body["subject"] = subject
			}
			if replyTo != "" {
				body["reply_to"] = replyTo
			}
			if html != "" {
				body["html"] = html
			}
			if text != "" {
				body["text"] = text
			}
			if name != "" {
				body["name"] = name
			}
			resp, err := s.call(cmd.Context(), key, http.MethodPatch, "/broadcasts/"+args[0], body, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&subject, "subject", "", "broadcast subject")
	cmd.Flags().StringVar(&replyTo, "reply-to", "", "reply-to address")
	cmd.Flags().StringVar(&html, "html", "", "HTML body")
	cmd.Flags().StringVar(&text, "text", "", "plain-text body")
	cmd.Flags().StringVar(&name, "name", "", "internal broadcast name")
	return cmd
}

func (s *Service) newBroadcastSendCmd(key string) *cobra.Command {
	var scheduledAt string
	cmd := &cobra.Command{
		Use:         "send <id>",
		Short:       "Send or schedule a broadcast (POST /broadcasts/{id}/send)",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{}
			if scheduledAt != "" {
				body["scheduled_at"] = scheduledAt
			}
			resp, err := s.call(cmd.Context(), key, http.MethodPost, "/broadcasts/"+args[0]+"/send", body, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&scheduledAt, "scheduled-at", "", "schedule time: ISO-8601 or natural language (immediate if omitted)")
	return cmd
}

func (s *Service) newBroadcastDeleteCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:         "delete <id>",
		Short:       "Delete a draft broadcast (DELETE /broadcasts/{id})",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), key, http.MethodDelete, "/broadcasts/"+args[0], nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}
