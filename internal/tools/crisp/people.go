package crisp

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newPeopleCmd builds the `crisp people` resource group (CRM contacts).
func (s *Service) newPeopleCmd(token string) *cobra.Command {
	group := &cobra.Command{
		Use:   "people",
		Short: "Website People (contacts / CRM profiles)",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	group.AddCommand(
		s.newPeopleListCmd(token),
		s.newPeopleGetCmd(token),
		s.newPeopleCreateCmd(token),
	)
	return group
}

func (s *Service) newPeopleListCmd(token string) *cobra.Command {
	var page int
	var search string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List or search People profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			website, err := websiteFlag(cmd)
			if err != nil {
				return err
			}
			query := url.Values{}
			if search != "" {
				query.Set("search_text", search)
			}
			path := fmt.Sprintf("/website/%s/people/profiles/%d", website, page)
			data, err := s.call(cmd.Context(), token, http.MethodGet, path, query, nil)
			if err != nil {
				return err
			}
			return s.emit(data, map[string]any{"website_id": website, "page": page})
		},
	}
	cmd.Flags().IntVar(&page, "page", 1, "page number (1-based)")
	cmd.Flags().StringVar(&search, "search", "", "free-text search over profiles")
	return cmd
}

func (s *Service) newPeopleGetCmd(token string) *cobra.Command {
	var people string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Look up one People profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			website, err := websiteFlag(cmd)
			if err != nil {
				return err
			}
			if _, err := requireFlag(cmd, "people"); err != nil {
				return err
			}
			path := fmt.Sprintf("/website/%s/people/profile/%s", website, people)
			data, err := s.call(cmd.Context(), token, http.MethodGet, path, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(data, map[string]any{"website_id": website, "people": people})
		},
	}
	cmd.Flags().StringVar(&people, "people", "", "people_id (required)")
	return cmd
}

func (s *Service) newPeopleCreateCmd(token string) *cobra.Command {
	var email, nickname string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Add a new People profile (contact)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			website, err := websiteFlag(cmd)
			if err != nil {
				return err
			}
			if _, err := requireFlag(cmd, "email"); err != nil {
				return err
			}
			body := map[string]any{"email": email}
			if nickname != "" {
				body["person"] = map[string]any{"nickname": nickname}
			}
			path := fmt.Sprintf("/website/%s/people/profile", website)
			data, err := s.call(cmd.Context(), token, http.MethodPost, path, nil, body)
			if err != nil {
				return err
			}
			return s.emit(data, map[string]any{"website_id": website})
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "contact email (required)")
	cmd.Flags().StringVar(&nickname, "nickname", "", "contact display name")
	return cmd
}
