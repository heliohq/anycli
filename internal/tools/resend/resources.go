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
		Use:   "list",
		Short: "List audiences (GET /audiences)",
		Long: "Audiences are the marketing lists a broadcast targets. This returns the\n" +
			"audiences and their ids; the people inside one come from\n" +
			"`contact list --audience <id>`, never from here.",
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
		Use:   "get <id>",
		Short: "Retrieve an audience (GET /audiences/{id})",
		Long: "Takes the audience id from `audience list`. Returns the audience record\n" +
			"itself — name and creation time — not its members, which are reached\n" +
			"with `contact list --audience <id>`.",
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
		Use:   "create",
		Short: "Create an audience (POST /audiences)",
		Long: "--name is the only property an audience has; there is no description or\n" +
			"nesting. The returned id is what every `contact` command takes as\n" +
			"--audience and what `broadcast create --audience` targets, so keep it.",
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
		Use:   "delete <id>",
		Short: "Delete an audience (DELETE /audiences/{id})",
		Long: "Removes the audience together with the contacts it holds, including\n" +
			"their unsubscribe state, and there is no undo or export command here —\n" +
			"a member list worth keeping has to be read out with `contact list`\n" +
			"first. Broadcasts already sent to it are unaffected.",
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
		Use:   "list",
		Short: "List contacts in an audience (GET /audiences/{aid}/contacts)",
		Long: "--audience is required because contacts exist only inside an audience:\n" +
			"the same person in two audiences is two independent records with\n" +
			"different ids and separate subscription state. Each entry carries the\n" +
			"`unsubscribed` flag that decides whether a broadcast reaches them.",
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
		Use:   "get <id>",
		Short: "Retrieve a contact by id or email (GET /audiences/{aid}/contacts/{id})",
		Long: "The positional argument accepts either the contact id or the email\n" +
			"address, so a lookup needs no prior list. --audience is still required,\n" +
			"and the same address in a different audience is a different record with\n" +
			"its own `unsubscribed` value.",
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
		Use:   "create",
		Short: "Create a contact (POST /audiences/{aid}/contacts)",
		Long: "--audience and --email are required. Adding an address sends nothing and\n" +
			"asks for no consent — the contact is subscribed unless --unsubscribed is\n" +
			"passed, so the legal basis for mailing them is entirely the caller's\n" +
			"problem. The same address may exist in several audiences independently.",
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
		Use:   "update <id>",
		Short: "Update a contact by id or email (PATCH /audiences/{aid}/contacts/{id})",
		Long: "Takes the contact id or email positionally and requires --audience. Only\n" +
			"flags actually passed are sent, and --unsubscribed in particular is\n" +
			"omitted unless given — which makes this the correct way to honour an\n" +
			"opt-out without disturbing the name fields. The change applies to this\n" +
			"audience alone.",
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
		Use:   "delete <id>",
		Short: "Delete a contact by id or email (DELETE /audiences/{aid}/contacts/{id})",
		Long: "Deleting is NOT unsubscribing: the record disappears together with its\n" +
			"unsubscribed flag, so the same address added again later is mailable\n" +
			"once more and the opt-out is lost. To honour an opt-out use\n" +
			"`contact update --unsubscribed` instead. --audience is required.",
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
		Use:   "list",
		Short: "List broadcasts (GET /broadcasts)",
		Long: "Returns broadcasts in every state — draft, scheduled and sent — so read\n" +
			"each `status` before assuming one went out. Creating a broadcast does\n" +
			"not send it, and a draft sits here indefinitely until `broadcast send`\n" +
			"is called on it.",
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
		Use:   "get <id>",
		Short: "Retrieve a broadcast (GET /broadcasts/{id})",
		Long: "Takes the broadcast id from `broadcast list`. Returns its content, target\n" +
			"audience, status and send timestamps. Per-recipient delivery is not\n" +
			"exposed here or anywhere else in this tool — a broadcast reports on\n" +
			"itself, not on the individual messages it produced.",
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
		Use:   "create",
		Short: "Create a broadcast (POST /broadcasts)",
		Long: "Creates a DRAFT and sends nothing; reaching the audience is the separate\n" +
			"`broadcast send`. --audience and --from are required, the from address\n" +
			"has to sit on a verified domain, and --html or --text carries the body.\n" +
			"--name is an internal label recipients never see, unlike --subject.",
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
		Use:   "update <id>",
		Short: "Update a draft broadcast (PATCH /broadcasts/{id})",
		Long: "Only a broadcast still in draft can be edited; one already sent rejects\n" +
			"the change. Fields not passed are left alone. The target audience is not\n" +
			"editable here at all — pointing the same content at a different audience\n" +
			"means creating a new broadcast.",
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
		Use:   "send <id>",
		Short: "Send or schedule a broadcast (POST /broadcasts/{id}/send)",
		Long: "The irreversible step: the draft goes to EVERY subscribed contact in its\n" +
			"audience. --scheduled-at queues it for later instead of sending\n" +
			"immediately, but no command in this tool cancels a broadcast once this\n" +
			"has been called — `email cancel` covers transactional sends only.\n" +
			"Deleting the draft beforehand is the only exit.",
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
		Use:   "delete <id>",
		Short: "Delete a draft broadcast (DELETE /broadcasts/{id})",
		Long: "Removes the broadcast record. It retracts nothing: mail already\n" +
			"delivered stays delivered, and deleting a sent broadcast only discards\n" +
			"the record that `broadcast list` was reporting on.",
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
