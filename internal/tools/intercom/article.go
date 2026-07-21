package intercom

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newArticleCmd builds the article resource group: the Help Center content an
// agent finds to link a customer, plus drafting/updating articles. Article
// search is a GET with a `phrase` query param (not the POST search-body model
// the inbox resources use).
func (s *Service) newArticleCmd(token string) *cobra.Command {
	cmd := newGroupCmd("article", "Help Center articles: list, get, search, create, update")
	cmd.AddCommand(
		s.newArticleListCmd(token),
		s.newArticleGetCmd(token),
		s.newArticleSearchCmd(token),
		s.newArticleCreateCmd(token),
		s.newArticleUpdateCmd(token),
		s.newArticleCollectionListCmd(token),
	)
	return cmd
}

func (s *Service) newArticleListCmd(token string) *cobra.Command {
	var perPage int
	var page int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List articles (GET /articles)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if perPage > 0 {
				q.Set("per_page", intToString(perPage))
			}
			if page > 0 {
				q.Set("page", intToString(page))
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/articles", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&perPage, "per-page", 0, "results per page")
	cmd.Flags().IntVar(&page, "page", 0, "page number")
	return cmd
}

func (s *Service) newArticleGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get one article (GET /articles/{id})",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/articles/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "article id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newArticleSearchCmd(token string) *cobra.Command {
	var phrase, state string
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search articles by phrase (GET /articles/search)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("phrase", phrase)
			if state != "" {
				q.Set("state", state)
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/articles/search", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&phrase, "phrase", "", "search phrase")
	cmd.Flags().StringVar(&state, "state", "", "filter by state (published|draft)")
	_ = cmd.MarkFlagRequired("phrase")
	return cmd
}

func (s *Service) newArticleCreateCmd(token string) *cobra.Command {
	var title, authorID, body, state, parentID, bodyJSON string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an article (POST /articles)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload := articleBody(title, authorID, body, state, parentID)
			if err := mergeBodyJSON(payload, bodyJSON); err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/articles", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerArticleFlags(cmd, &title, &authorID, &body, &state, &parentID, &bodyJSON)
	_ = cmd.MarkFlagRequired("title")
	_ = cmd.MarkFlagRequired("author-id")
	return cmd
}

func (s *Service) newArticleUpdateCmd(token string) *cobra.Command {
	var id, title, authorID, body, state, parentID, bodyJSON string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update an article (PUT /articles/{id})",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload := articleBody(title, authorID, body, state, parentID)
			if err := mergeBodyJSON(payload, bodyJSON); err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPut, "/articles/"+url.PathEscape(id), nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "article id")
	registerArticleFlags(cmd, &title, &authorID, &body, &state, &parentID, &bodyJSON)
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newArticleCollectionListCmd(token string) *cobra.Command {
	var perPage int
	var page int
	cmd := &cobra.Command{
		Use:   "collection-list",
		Short: "List Help Center collections (GET /help_center/collections)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if perPage > 0 {
				q.Set("per_page", intToString(perPage))
			}
			if page > 0 {
				q.Set("page", intToString(page))
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/help_center/collections", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&perPage, "per-page", 0, "results per page")
	cmd.Flags().IntVar(&page, "page", 0, "page number")
	return cmd
}

// registerArticleFlags wires the shared create/update article flags.
func registerArticleFlags(cmd *cobra.Command, title, authorID, body, state, parentID, bodyJSON *string) {
	cmd.Flags().StringVar(title, "title", "", "article title")
	cmd.Flags().StringVar(authorID, "author-id", "", "authoring admin id")
	cmd.Flags().StringVar(body, "body", "", "article body (HTML)")
	cmd.Flags().StringVar(state, "state", "", "article state (published|draft)")
	cmd.Flags().StringVar(parentID, "parent-id", "", "parent collection/section id")
	cmd.Flags().StringVar(bodyJSON, "body-json", "", "raw article JSON (merged; overrides the scalar flags)")
}

// articleBody assembles a create/update article payload from scalar flags.
func articleBody(title, authorID, body, state, parentID string) map[string]any {
	payload := map[string]any{}
	if title != "" {
		payload["title"] = title
	}
	if authorID != "" {
		payload["author_id"] = authorID
	}
	if body != "" {
		payload["body"] = body
	}
	if state != "" {
		payload["state"] = state
	}
	if parentID != "" {
		payload["parent_id"] = parentID
	}
	return payload
}
