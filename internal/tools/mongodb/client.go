package mongodb

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// FindQuery carries the per-invocation read options for Find.
type FindQuery struct {
	Filter     bson.D
	Sort       bson.D
	Projection bson.D
	Limit      int64
	Skip       int64
}

// Client is the driver seam. The zero-value Service connects with the real
// mongo driver; tests inject a fake through Service.Connect.
type Client interface {
	Ping(ctx context.Context) error
	ListDatabaseNames(ctx context.Context) ([]string, error)
	ListCollectionNames(ctx context.Context, db string) ([]string, error)
	ListIndexes(ctx context.Context, db, coll string) ([]bson.M, error)
	Find(ctx context.Context, db, coll string, q FindQuery) ([]bson.M, error)
	Count(ctx context.Context, db, coll string, filter bson.D) (int64, error)
	Aggregate(ctx context.Context, db, coll string, pipeline bson.A) ([]bson.M, error)
	InsertMany(ctx context.Context, db, coll string, docs []any) ([]any, error)
	Update(ctx context.Context, db, coll string, filter bson.D, update bson.D, many, upsert bool) (*mongo.UpdateResult, error)
	Delete(ctx context.Context, db, coll string, filter bson.D, many bool) (int64, error)
	Disconnect(ctx context.Context) error
}

// driverConnect is the production Client constructor. mongo.Connect does not
// dial; connection and auth errors surface on the first operation.
func driverConnect(_ context.Context, uri string) (Client, error) {
	c, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("mongodb: connect: %w", err)
	}
	return &driverClient{c: c}, nil
}

type driverClient struct {
	c *mongo.Client
}

func (d *driverClient) Ping(ctx context.Context) error {
	return d.c.Ping(ctx, nil)
}

func (d *driverClient) ListDatabaseNames(ctx context.Context) ([]string, error) {
	return d.c.ListDatabaseNames(ctx, bson.D{})
}

func (d *driverClient) ListCollectionNames(ctx context.Context, db string) ([]string, error) {
	return d.c.Database(db).ListCollectionNames(ctx, bson.D{})
}

func (d *driverClient) ListIndexes(ctx context.Context, db, coll string) ([]bson.M, error) {
	cur, err := d.c.Database(db).Collection(coll).Indexes().List(ctx)
	if err != nil {
		return nil, err
	}
	var out []bson.M
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (d *driverClient) Find(ctx context.Context, db, coll string, q FindQuery) ([]bson.M, error) {
	opts := options.Find()
	if q.Limit > 0 {
		opts.SetLimit(q.Limit)
	}
	if q.Skip > 0 {
		opts.SetSkip(q.Skip)
	}
	if q.Sort != nil {
		opts.SetSort(q.Sort)
	}
	if q.Projection != nil {
		opts.SetProjection(q.Projection)
	}
	cur, err := d.c.Database(db).Collection(coll).Find(ctx, q.Filter, opts)
	if err != nil {
		return nil, err
	}
	var out []bson.M
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (d *driverClient) Count(ctx context.Context, db, coll string, filter bson.D) (int64, error) {
	return d.c.Database(db).Collection(coll).CountDocuments(ctx, filter)
}

func (d *driverClient) Aggregate(ctx context.Context, db, coll string, pipeline bson.A) ([]bson.M, error) {
	cur, err := d.c.Database(db).Collection(coll).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	var out []bson.M
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (d *driverClient) InsertMany(ctx context.Context, db, coll string, docs []any) ([]any, error) {
	res, err := d.c.Database(db).Collection(coll).InsertMany(ctx, docs)
	if err != nil {
		return nil, err
	}
	return res.InsertedIDs, nil
}

func (d *driverClient) Update(ctx context.Context, db, coll string, filter bson.D, update bson.D, many, upsert bool) (*mongo.UpdateResult, error) {
	c := d.c.Database(db).Collection(coll)
	if many {
		return c.UpdateMany(ctx, filter, update, options.UpdateMany().SetUpsert(upsert))
	}
	return c.UpdateOne(ctx, filter, update, options.UpdateOne().SetUpsert(upsert))
}

func (d *driverClient) Delete(ctx context.Context, db, coll string, filter bson.D, many bool) (int64, error) {
	c := d.c.Database(db).Collection(coll)
	var res *mongo.DeleteResult
	var err error
	if many {
		res, err = c.DeleteMany(ctx, filter)
	} else {
		res, err = c.DeleteOne(ctx, filter)
	}
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

func (d *driverClient) Disconnect(ctx context.Context) error {
	return d.c.Disconnect(ctx)
}
