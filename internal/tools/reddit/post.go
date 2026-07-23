package reddit

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

// newPostCmd groups post operations: get, comments, create, edit, delete.
func (s *Service) newPostCmd(token string) *cobra.Command {
	cmd := newGroup("post", "Read and write posts")
	cmd.AddCommand(
		s.newPostGetCmd(token),
		s.newPostCommentsCmd(token),
		s.newPostCreateCmd(token),
		s.newPostEditCmd(token),
		s.newPostDeleteCmd(token),
	)
	return cmd
}

func (s *Service) newPostGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <id>",
		Short:       "Fetch a single post by id (t3_ prefix optional)",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			if !hasPrefix(id) {
				id = "t3_" + id
			}
			body, err := s.get(cmd.Context(), token, "/api/info", url.Values{"id": {id}})
			if err != nil {
				return err
			}
			return s.emitPostListing(jsonFlag(jsonMode(cmd)), body)
		},
	}
}

func (s *Service) newPostCommentsCmd(token string) *cobra.Command {
	var sort string
	var depth, limit int
	cmd := &cobra.Command{
		Use:         "comments <id>",
		Short:       "Fetch a post's comment tree (flattened, with depth)",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireEnum("sort", sort, "best", "top", "new", "controversial", "old", "qa"); err != nil {
				return err
			}
			if err := requireLimit(limit); err != nil {
				return err
			}
			q := url.Values{}
			if sort != "" {
				q.Set("sort", sort)
			}
			if depth != 0 {
				q.Set("depth", intToStr(depth))
			}
			if limit != 0 {
				q.Set("limit", intToStr(limit))
			}
			body, err := s.get(cmd.Context(), token, "/comments/"+url.PathEscape(articleID(args[0])), q)
			if err != nil {
				return err
			}
			return s.emitCommentTree(jsonFlag(jsonMode(cmd)), body)
		},
	}
	cmd.Flags().StringVar(&sort, "sort", "", "best|top|new|controversial|old|qa")
	cmd.Flags().IntVar(&depth, "depth", 0, "maximum comment nesting depth")
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum comments to return")
	return cmd
}

func (s *Service) newPostCreateCmd(token string) *cobra.Command {
	var subreddit, title, text, link string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Submit a text or link post",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if subreddit == "" || title == "" {
				return &usageError{msg: "--subreddit and --title are required"}
			}
			if (text == "") == (link == "") {
				return &usageError{msg: "exactly one of --text or --url is required"}
			}
			form := url.Values{
				"api_type": {"json"},
				"sr":       {subreddit},
				"title":    {title},
			}
			if text != "" {
				form.Set("kind", "self")
				form.Set("text", text)
			} else {
				form.Set("kind", "link")
				form.Set("url", link)
			}
			body, err := s.postForm(cmd.Context(), token, "/api/submit", form)
			if err != nil {
				return err
			}
			env, err := checkJSONErrors(body)
			if err != nil {
				return err
			}
			return s.emitCreated(cmd, env.JSON.Data.ID, env.JSON.Data.Name, env.JSON.Data.URL)
		},
	}
	cmd.Flags().StringVar(&subreddit, "subreddit", "", "target subreddit (without r/) (required)")
	cmd.Flags().StringVar(&title, "title", "", "post title (required)")
	cmd.Flags().StringVar(&text, "text", "", "self-post body (markdown); mutually exclusive with --url")
	cmd.Flags().StringVar(&link, "url", "", "link URL; mutually exclusive with --text")
	return cmd
}

func (s *Service) newPostEditCmd(token string) *cobra.Command {
	var text string
	cmd := &cobra.Command{
		Use:         "edit <fullname>",
		Short:       "Edit the body of your own self-post (t3_ fullname)",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireFullname(args[0]); err != nil {
				return err
			}
			if text == "" {
				return &usageError{msg: "--text is required"}
			}
			return s.editUserText(cmd, token, args[0], text)
		},
	}
	cmd.Flags().StringVar(&text, "text", "", "new body markdown (required)")
	return cmd
}

func (s *Service) newPostDeleteCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "delete <fullname>",
		Short:       "Delete your own post (t3_ fullname)",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.deleteThing(cmd, token, args[0])
		},
	}
}

// emitCreated echoes a newly created thing (post or comment).
func (s *Service) emitCreated(cmd *cobra.Command, id, fullname, permalink string) error {
	if jsonMode(cmd) {
		out := map[string]any{"id": id, "fullname": fullname}
		if permalink != "" {
			out["permalink"] = permalink
		}
		return s.emitValue(out)
	}
	return s.emitLine(fmt.Sprintf("created %s\t%s", fullname, permalink))
}

// editUserText edits the body of an owned self-post or comment.
func (s *Service) editUserText(cmd *cobra.Command, token, fullname, text string) error {
	form := url.Values{"api_type": {"json"}, "thing_id": {fullname}, "text": {text}}
	body, err := s.postForm(cmd.Context(), token, "/api/editusertext", form)
	if err != nil {
		return err
	}
	env, err := checkJSONErrors(body)
	if err != nil {
		return err
	}
	if d, ok := createdThing(env); ok {
		return s.emitCreated(cmd, d.ID, d.Name, d.Permalink)
	}
	return s.emitCreated(cmd, "", fullname, "")
}

// deleteThing removes an owned post or comment by fullname.
func (s *Service) deleteThing(cmd *cobra.Command, token, fullname string) error {
	if err := requireFullname(fullname); err != nil {
		return err
	}
	if _, err := s.postForm(cmd.Context(), token, "/api/del", url.Values{"id": {fullname}}); err != nil {
		return err
	}
	if jsonMode(cmd) {
		return s.emitValue(map[string]any{"deleted": fullname})
	}
	return s.emitLine("deleted " + fullname)
}

// hasPrefix reports whether an id already carries a t?_ fullname prefix.
func hasPrefix(id string) bool {
	return len(id) > 3 && id[0] == 't' && id[2] == '_'
}
