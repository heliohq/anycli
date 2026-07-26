package gmail

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newLabelsListCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List labels",
		Long: "System labels (`INBOX`, `UNREAD`, `STARRED`, `SENT`, `SPAM`, `TRASH`) have\n" +
			"`type` system and their id IS the name; user-created labels have opaque\n" +
			"`Label_…` ids whose display name is something else entirely.\n" +
			"`messages modify` and `messages list --label` both take the id, so this is\n" +
			"the lookup that has to happen first. Not paginated — the whole set arrives\n" +
			"in one call.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/users/me/labels", nil, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				Labels []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
					Type string `json:"type"`
				} `json:"labels"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("gmail: decode label list: %w", err)
			}
			for _, l := range resp.Labels {
				fmt.Fprintf(s.stdout(), "%s\t%s\t%s\n", l.ID, l.Name, l.Type)
			}
			return nil
		},
	}
}

func (s *Service) newLabelsGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <label-id>",
		Short: "Show one label with its message/thread counters (e.g. `labels get INBOX` — messagesUnread is the inbox unread count, no pagination needed)",
		Long: "Messages and threads are counted separately, each with a total and an unread\n" +
			"figure, so \"12 unread\" as a message count and as a conversation count are\n" +
			"different numbers from the same reply. A nested label reports only its own\n" +
			"contents — counters do not roll up from child labels. `type` separates\n" +
			"Gmail's own labels from user-created ones.",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/users/me/labels/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var l struct {
				ID             string `json:"id"`
				Name           string `json:"name"`
				Type           string `json:"type"`
				MessagesTotal  int64  `json:"messagesTotal"`
				MessagesUnread int64  `json:"messagesUnread"`
				ThreadsTotal   int64  `json:"threadsTotal"`
				ThreadsUnread  int64  `json:"threadsUnread"`
			}
			if err := json.Unmarshal(body, &l); err != nil {
				return fmt.Errorf("gmail: decode label: %w", err)
			}
			fmt.Fprintf(s.stdout(),
				"Id:              %s\nName:            %s\nType:            %s\nMessagesTotal:   %d\nMessagesUnread:  %d\nThreadsTotal:    %d\nThreadsUnread:   %d\n",
				l.ID, l.Name, l.Type, l.MessagesTotal, l.MessagesUnread, l.ThreadsTotal, l.ThreadsUnread)
			return nil
		},
	}
}

func (s *Service) newLabelsCreateCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "create <name>",
		Short: "Create a label",
		Long: "The argument is the display name, and nesting lives in the name itself: a\n" +
			"label called `Clients/Acme` appears under `Clients` in Gmail. Gmail rejects\n" +
			"a name that already exists rather than handing back the existing label, so\n" +
			"check `labels list` first. The response carries the `Label_…` id that\n" +
			"`messages modify --add-label` needs. Labels cannot be renamed or deleted\n" +
			"from here.",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/users/me/labels", nil, map[string]string{"name": args[0]})
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var label struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			}
			if err := json.Unmarshal(body, &label); err != nil {
				return fmt.Errorf("gmail: decode label: %w", err)
			}
			fmt.Fprintf(s.stdout(), "created label %s (%s)\n", label.Name, label.ID)
			return nil
		},
	}
}
