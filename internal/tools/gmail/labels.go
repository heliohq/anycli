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
		Use:         "list",
		Short:       "List labels",
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
		Use:         "get <label-id>",
		Short:       "Show one label with its message/thread counters (e.g. `labels get INBOX` — messagesUnread is the inbox unread count, no pagination needed)",
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
		Use:         "create <name>",
		Short:       "Create a label",
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
