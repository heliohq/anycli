package knock

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newObjectCmd groups the object verbs. Objects are non-user recipients (a
// project, document, or account) that can receive notifications and be
// subscribed to. They live in named collections.
func (s *Service) newObjectCmd(key string) *cobra.Command {
	group := newGroupCmd("object", "Manage non-user recipients (objects) in a collection")
	group.AddCommand(
		s.newObjectSetCmd(key),
		s.newObjectGetCmd(key),
		s.newObjectDeleteCmd(key),
		s.newObjectListCmd(key),
		s.newObjectSubscriptionsCmd(key),
	)
	return group
}

// objectPath builds /objects/{collection}/{id} with both segments validated.
func objectPath(collection, id string) (string, error) {
	if err := requireID("collection", collection); err != nil {
		return "", err
	}
	if err := requireID("id", id); err != nil {
		return "", err
	}
	return "/objects/" + url.PathEscape(collection) + "/" + url.PathEscape(id), nil
}

func (s *Service) newObjectSetCmd(key string) *cobra.Command {
	var (
		collection string
		id         string
		data       string
	)
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set (create or update) an object",
		Long: "--collection and --id are both required: an object id is unique only inside\n" +
			"its collection. This is an upsert, so calling it again with different\n" +
			"--data replaces the object's properties instead of failing. Objects are\n" +
			"recipients that are not people — a project, a document, an account — and\n" +
			"they become notifiable through the users subscribed to them.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := objectPath(collection, id)
			if err != nil {
				return err
			}
			body := map[string]any{}
			if data != "" {
				parsed, decErr := decodeJSONFlag("data", data)
				if decErr != nil {
					return decErr
				}
				obj, ok := parsed.(map[string]any)
				if !ok {
					return &usageError{msg: "--data must be a JSON object"}
				}
				body = obj
			}
			return s.callEmit(cmd.Context(), key, http.MethodPut, path, nil, body, nil)
		},
	}
	cmd.Flags().StringVar(&collection, "collection", "", "object collection name (required)")
	cmd.Flags().StringVar(&id, "id", "", "object id (required)")
	cmd.Flags().StringVar(&data, "data", "", "object properties as a JSON object")
	return cmd
}

func (s *Service) newObjectGetCmd(key string) *cobra.Command {
	var (
		collection string
		id         string
	)
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get an object",
		Long: "Requires both --collection and --id; there is no lookup by id alone.\n" +
			"Returns the object's own properties, not the users subscribed to it —\n" +
			"those come from `object subscriptions`.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := objectPath(collection, id)
			if err != nil {
				return err
			}
			return s.callEmit(cmd.Context(), key, http.MethodGet, path, nil, nil, nil)
		},
	}
	cmd.Flags().StringVar(&collection, "collection", "", "object collection name (required)")
	cmd.Flags().StringVar(&id, "id", "", "object id (required)")
	return cmd
}

func (s *Service) newObjectDeleteCmd(key string) *cobra.Command {
	var (
		collection string
		id         string
	)
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete an object",
		Long: "Requires --collection and --id. Removing the object removes it as a\n" +
			"recipient, so a workflow that fanned out through its subscriptions stops\n" +
			"reaching anyone that way. Nothing here restores it, and there is no\n" +
			"command to remove a single subscription instead.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := objectPath(collection, id)
			if err != nil {
				return err
			}
			return s.callEmit(cmd.Context(), key, http.MethodDelete, path, nil, nil, nil)
		},
	}
	cmd.Flags().StringVar(&collection, "collection", "", "object collection name (required)")
	cmd.Flags().StringVar(&id, "id", "", "object id (required)")
	return cmd
}

func (s *Service) newObjectListCmd(key string) *cobra.Command {
	var (
		collection string
		pageSize   int
		after      string
		before     string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List objects in a collection",
		Long: "--collection is required: objects are always listed within one collection\n" +
			"and there is no cross-collection listing, nor any command that enumerates\n" +
			"the collections themselves. Paged with --page-size (Knock's default is 50)\n" +
			"and --after.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID("collection", collection); err != nil {
				return err
			}
			q := url.Values{}
			addPaging(q, pageSize, after, before)
			return s.callEmit(cmd.Context(), key, http.MethodGet, "/objects/"+url.PathEscape(collection), q, nil, nil)
		},
	}
	cmd.Flags().StringVar(&collection, "collection", "", "object collection name (required)")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "page size (Knock default 50)")
	cmd.Flags().StringVar(&after, "after", "", "pagination cursor (next page)")
	cmd.Flags().StringVar(&before, "before", "", "pagination cursor (previous page)")
	return cmd
}

func (s *Service) newObjectSubscriptionsCmd(key string) *cobra.Command {
	var (
		collection string
		id         string
		pageSize   int
		after      string
		before     string
	)
	cmd := &cobra.Command{
		Use:   "subscriptions",
		Short: "List an object's subscriptions (who follows it)",
		Long: "The recipients subscribed to this object, which is the audience a workflow\n" +
			"reaches when it fans out through the object rather than through named\n" +
			"recipients. Requires --collection and --id. This tool can read\n" +
			"subscriptions but neither create nor remove one.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := objectPath(collection, id)
			if err != nil {
				return err
			}
			q := url.Values{}
			addPaging(q, pageSize, after, before)
			return s.callEmit(cmd.Context(), key, http.MethodGet, path+"/subscriptions", q, nil, nil)
		},
	}
	cmd.Flags().StringVar(&collection, "collection", "", "object collection name (required)")
	cmd.Flags().StringVar(&id, "id", "", "object id (required)")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "page size (Knock default 50)")
	cmd.Flags().StringVar(&after, "after", "", "pagination cursor (next page)")
	cmd.Flags().StringVar(&before, "before", "", "pagination cursor (previous page)")
	return cmd
}
