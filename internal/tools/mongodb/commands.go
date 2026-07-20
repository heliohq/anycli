package mongodb

import (
	"fmt"

	"github.com/spf13/cobra"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func (s *Service) newRoot(dsn string) *cobra.Command {
	root := &cobra.Command{
		Use:           "mongodb",
		Short:         "MongoDB built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")
	// --db is per-invocation; the DSN path component is only its default.
	root.PersistentFlags().String("db", "", "target database (default: the connection string's database, if any)")

	databases := &cobra.Command{Use: "databases", Short: "Databases"}
	databases.AddCommand(s.newDatabasesListCmd(dsn))

	collections := &cobra.Command{Use: "collections", Short: "Collections"}
	collections.AddCommand(s.newCollectionsListCmd(dsn))

	indexes := &cobra.Command{Use: "indexes", Short: "Indexes"}
	indexes.AddCommand(s.newIndexesListCmd(dsn))

	root.AddCommand(
		s.newPingCmd(dsn),
		databases,
		collections,
		indexes,
		s.newFindCmd(dsn),
		s.newCountCmd(dsn),
		s.newAggregateCmd(dsn),
		s.newInsertCmd(dsn),
		s.newUpdateCmd(dsn),
		s.newDeleteCmd(dsn),
	)
	return root
}

// targetDB reads the persistent --db flag and resolves it against the DSN.
func targetDB(cmd *cobra.Command, dsn string) (string, error) {
	flagDB, err := cmd.Flags().GetString("db")
	if err != nil {
		return "", err
	}
	return resolveDB(flagDB, dsn)
}

func (s *Service) newPingCmd(dsn string) *cobra.Command {
	return &cobra.Command{
		Use:   "ping",
		Short: "Verify connectivity and authentication",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.withClient(cmd.Context(), dsn, func(c Client) error {
				if err := c.Ping(cmd.Context()); err != nil {
					return err
				}
				return s.emit(bson.M{"ok": true})
			})
		},
	}
}

func (s *Service) newDatabasesListCmd(dsn string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List database names",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.withClient(cmd.Context(), dsn, func(c Client) error {
				names, err := c.ListDatabaseNames(cmd.Context())
				if err != nil {
					return err
				}
				return s.emit(bson.M{"databases": names})
			})
		},
	}
}

func (s *Service) newCollectionsListCmd(dsn string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List a database's collection names",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			db, err := targetDB(cmd, dsn)
			if err != nil {
				return err
			}
			return s.withClient(cmd.Context(), dsn, func(c Client) error {
				names, err := c.ListCollectionNames(cmd.Context(), db)
				if err != nil {
					return err
				}
				return s.emit(bson.M{"database": db, "collections": names})
			})
		},
	}
}

func (s *Service) newIndexesListCmd(dsn string) *cobra.Command {
	var collection string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List a collection's indexes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			db, err := targetDB(cmd, dsn)
			if err != nil {
				return err
			}
			return s.withClient(cmd.Context(), dsn, func(c Client) error {
				indexes, err := c.ListIndexes(cmd.Context(), db, collection)
				if err != nil {
					return err
				}
				return s.emit(bson.M{"indexes": indexes})
			})
		},
	}
	cmd.Flags().StringVar(&collection, "collection", "", "collection name")
	_ = cmd.MarkFlagRequired("collection")
	return cmd
}

func (s *Service) newFindCmd(dsn string) *cobra.Command {
	var collection, filter, sort, projection string
	var limit, skip int64
	cmd := &cobra.Command{
		Use:   "find",
		Short: "Find documents matching a filter",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			db, err := targetDB(cmd, dsn)
			if err != nil {
				return err
			}
			q := FindQuery{Limit: limit, Skip: skip}
			if q.Filter, err = parseDocFlag("filter", filter); err != nil {
				return err
			}
			if sort != "" {
				if q.Sort, err = parseDocFlag("sort", sort); err != nil {
					return err
				}
			}
			if projection != "" {
				if q.Projection, err = parseDocFlag("projection", projection); err != nil {
					return err
				}
			}
			return s.withClient(cmd.Context(), dsn, func(c Client) error {
				docs, err := c.Find(cmd.Context(), db, collection, q)
				if err != nil {
					return err
				}
				return s.emit(bson.M{"documents": docs, "count": len(docs)})
			})
		},
	}
	cmd.Flags().StringVar(&collection, "collection", "", "collection name")
	cmd.Flags().StringVar(&filter, "filter", "{}", "query filter (extended JSON)")
	cmd.Flags().StringVar(&sort, "sort", "", "sort specification (extended JSON)")
	cmd.Flags().StringVar(&projection, "projection", "", "projection (extended JSON)")
	cmd.Flags().Int64Var(&limit, "limit", 0, "maximum documents to return (0 = no limit)")
	cmd.Flags().Int64Var(&skip, "skip", 0, "documents to skip")
	_ = cmd.MarkFlagRequired("collection")
	return cmd
}

func (s *Service) newCountCmd(dsn string) *cobra.Command {
	var collection, filter string
	cmd := &cobra.Command{
		Use:   "count",
		Short: "Count documents matching a filter",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			db, err := targetDB(cmd, dsn)
			if err != nil {
				return err
			}
			f, err := parseDocFlag("filter", filter)
			if err != nil {
				return err
			}
			return s.withClient(cmd.Context(), dsn, func(c Client) error {
				n, err := c.Count(cmd.Context(), db, collection, f)
				if err != nil {
					return err
				}
				return s.emit(bson.M{"count": n})
			})
		},
	}
	cmd.Flags().StringVar(&collection, "collection", "", "collection name")
	cmd.Flags().StringVar(&filter, "filter", "{}", "query filter (extended JSON)")
	_ = cmd.MarkFlagRequired("collection")
	return cmd
}

func (s *Service) newAggregateCmd(dsn string) *cobra.Command {
	var collection, pipeline string
	cmd := &cobra.Command{
		Use:   "aggregate",
		Short: "Run an aggregation pipeline",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			db, err := targetDB(cmd, dsn)
			if err != nil {
				return err
			}
			p, err := parseArrayFlag("pipeline", pipeline)
			if err != nil {
				return err
			}
			return s.withClient(cmd.Context(), dsn, func(c Client) error {
				docs, err := c.Aggregate(cmd.Context(), db, collection, p)
				if err != nil {
					return err
				}
				return s.emit(bson.M{"documents": docs, "count": len(docs)})
			})
		},
	}
	cmd.Flags().StringVar(&collection, "collection", "", "collection name")
	cmd.Flags().StringVar(&pipeline, "pipeline", "", "aggregation pipeline (extended JSON array)")
	_ = cmd.MarkFlagRequired("collection")
	_ = cmd.MarkFlagRequired("pipeline")
	return cmd
}

func (s *Service) newInsertCmd(dsn string) *cobra.Command {
	var collection string
	var docs []string
	cmd := &cobra.Command{
		Use:   "insert",
		Short: "Insert one or more documents",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			db, err := targetDB(cmd, dsn)
			if err != nil {
				return err
			}
			parsed := make([]any, 0, len(docs))
			for _, raw := range docs {
				d, err := parseDocFlag("doc", raw)
				if err != nil {
					return err
				}
				parsed = append(parsed, d)
			}
			return s.withClient(cmd.Context(), dsn, func(c Client) error {
				ids, err := c.InsertMany(cmd.Context(), db, collection, parsed)
				if err != nil {
					return err
				}
				return s.emit(bson.M{"inserted_count": len(ids), "inserted_ids": ids})
			})
		},
	}
	cmd.Flags().StringVar(&collection, "collection", "", "collection name")
	cmd.Flags().StringArrayVar(&docs, "doc", nil, "document to insert (extended JSON; repeatable)")
	_ = cmd.MarkFlagRequired("collection")
	_ = cmd.MarkFlagRequired("doc")
	return cmd
}

func (s *Service) newUpdateCmd(dsn string) *cobra.Command {
	var collection, filter, update string
	var many, upsert bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update documents matching a filter",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			db, err := targetDB(cmd, dsn)
			if err != nil {
				return err
			}
			f, err := parseDocFlag("filter", filter)
			if err != nil {
				return err
			}
			u, err := parseDocFlag("update", update)
			if err != nil {
				return err
			}
			return s.withClient(cmd.Context(), dsn, func(c Client) error {
				res, err := c.Update(cmd.Context(), db, collection, f, u, many, upsert)
				if err != nil {
					return err
				}
				out := bson.M{
					"matched_count":  res.MatchedCount,
					"modified_count": res.ModifiedCount,
					"upserted_count": res.UpsertedCount,
				}
				if res.UpsertedID != nil {
					out["upserted_id"] = res.UpsertedID
				}
				return s.emit(out)
			})
		},
	}
	cmd.Flags().StringVar(&collection, "collection", "", "collection name")
	cmd.Flags().StringVar(&filter, "filter", "", "query filter (extended JSON)")
	cmd.Flags().StringVar(&update, "update", "", "update document, e.g. {\"$set\": {...}} (extended JSON)")
	cmd.Flags().BoolVar(&many, "many", false, "update all matching documents (default: first match)")
	cmd.Flags().BoolVar(&upsert, "upsert", false, "insert when no document matches")
	_ = cmd.MarkFlagRequired("collection")
	_ = cmd.MarkFlagRequired("filter")
	_ = cmd.MarkFlagRequired("update")
	return cmd
}

func (s *Service) newDeleteCmd(dsn string) *cobra.Command {
	var collection, filter string
	var many bool
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete documents matching a filter",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			db, err := targetDB(cmd, dsn)
			if err != nil {
				return err
			}
			f, err := parseDocFlag("filter", filter)
			if err != nil {
				return err
			}
			return s.withClient(cmd.Context(), dsn, func(c Client) error {
				n, err := c.Delete(cmd.Context(), db, collection, f, many)
				if err != nil {
					return err
				}
				return s.emit(bson.M{"deleted_count": n})
			})
		},
	}
	cmd.Flags().StringVar(&collection, "collection", "", "collection name")
	cmd.Flags().StringVar(&filter, "filter", "", "query filter (extended JSON)")
	cmd.Flags().BoolVar(&many, "many", false, "delete all matching documents (default: first match)")
	_ = cmd.MarkFlagRequired("collection")
	_ = cmd.MarkFlagRequired("filter")
	return cmd
}

// parseDocFlag decodes one extended-JSON document flag into an order-preserving
// bson.D (sort specifications depend on key order).
func parseDocFlag(flag, value string) (bson.D, error) {
	var d bson.D
	if err := bson.UnmarshalExtJSON([]byte(value), false, &d); err != nil {
		return nil, fmt.Errorf("invalid --%s: %w", flag, err)
	}
	return d, nil
}

// parseArrayFlag decodes an extended-JSON array flag. The value is wrapped in
// a document because the extended-JSON decoder expects a document at the top
// level.
func parseArrayFlag(flag, value string) (bson.A, error) {
	var wrapper struct {
		Value bson.A `bson:"value"`
	}
	if err := bson.UnmarshalExtJSON([]byte(`{"value":`+value+`}`), false, &wrapper); err != nil {
		return nil, fmt.Errorf("invalid --%s: %w", flag, err)
	}
	return wrapper.Value, nil
}
