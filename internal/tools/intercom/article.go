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
		Long: "Page-numbered with --page rather than cursor-based, and it returns drafts\n" +
			"alongside published articles, so state has to be read off each item.\n" +
			"Looking for an article by topic is `article search`, which is a much\n" +
			"smaller response.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "The only way to read an article's full HTML body — list and search results\n" +
			"carry the metadata, not the whole document. Ids come from `article list`\n" +
			"or `article search`.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "A plain phrase GET over the Help Center, structurally unlike the inbox\n" +
			"searches: --phrase is required, there is no --query grammar, no field\n" +
			"operators and no cursor pagination. --state narrows to published or draft,\n" +
			"which matters because an unset state returns drafts that no customer can\n" +
			"actually open.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "--author-id is required and must be an admin id from `admin list` that\n" +
			"holds Help Center authoring rights. --body is HTML, not markdown. Leaving\n" +
			"--state unset creates a draft; passing published makes the article\n" +
			"publicly readable on the Help Center the moment the call returns, and\n" +
			"there is no delete to undo that. --parent-id files it under a collection\n" +
			"or section id from `article collection-list`.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
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
		Long: "Only the flags passed are sent, so omitted fields keep their values — but\n" +
			"--body is whole-document: it REPLACES the entire HTML rather than patching\n" +
			"it, so read the current body with `article get` before rewriting. Setting\n" +
			"--state published publishes a draft immediately.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
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
		Long: "Supplies the ids `article create --parent-id` and `article update\n" +
			"--parent-id` take. Collections and their sections are the Help Center's\n" +
			"folder structure; creating one is not exposed here, so an article can only\n" +
			"be filed under a collection that already exists. Page-numbered with\n" +
			"--page.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
