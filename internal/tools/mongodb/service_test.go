package mongodb

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// fakeClient records the last call and returns canned data or a fixed error.
type fakeClient struct {
	err error

	db, coll     string
	filter       bson.D
	query        FindQuery
	pipeline     bson.A
	docs         []any
	update       bson.D
	many, upsert bool

	databases    []string
	collections  []string
	found        []bson.M
	count        int64
	insertedIDs  []any
	updateResult *mongo.UpdateResult
	deleted      int64
	disconnected bool
}

func (f *fakeClient) Ping(context.Context) error { return f.err }

func (f *fakeClient) ListDatabaseNames(context.Context) ([]string, error) {
	return f.databases, f.err
}

func (f *fakeClient) ListCollectionNames(_ context.Context, db string) ([]string, error) {
	f.db = db
	return f.collections, f.err
}

func (f *fakeClient) ListIndexes(_ context.Context, db, coll string) ([]bson.M, error) {
	f.db, f.coll = db, coll
	return f.found, f.err
}

func (f *fakeClient) Find(_ context.Context, db, coll string, q FindQuery) ([]bson.M, error) {
	f.db, f.coll, f.query = db, coll, q
	return f.found, f.err
}

func (f *fakeClient) Count(_ context.Context, db, coll string, filter bson.D) (int64, error) {
	f.db, f.coll, f.filter = db, coll, filter
	return f.count, f.err
}

func (f *fakeClient) Aggregate(_ context.Context, db, coll string, pipeline bson.A) ([]bson.M, error) {
	f.db, f.coll, f.pipeline = db, coll, pipeline
	return f.found, f.err
}

func (f *fakeClient) InsertMany(_ context.Context, db, coll string, docs []any) ([]any, error) {
	f.db, f.coll, f.docs = db, coll, docs
	return f.insertedIDs, f.err
}

func (f *fakeClient) Update(_ context.Context, db, coll string, filter, update bson.D, many, upsert bool) (*mongo.UpdateResult, error) {
	f.db, f.coll, f.filter, f.update, f.many, f.upsert = db, coll, filter, update, many, upsert
	return f.updateResult, f.err
}

func (f *fakeClient) Delete(_ context.Context, db, coll string, filter bson.D, many bool) (int64, error) {
	f.db, f.coll, f.filter, f.many = db, coll, filter, many
	return f.deleted, f.err
}

func (f *fakeClient) Disconnect(context.Context) error {
	f.disconnected = true
	return nil
}

const testDSN = "mongodb+srv://appuser:s3cr3t-pw@cluster0.example.mongodb.net/?retryWrites=true"

func run(t *testing.T, fake *fakeClient, dsn string, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	svc := &Service{
		Connect: func(context.Context, string) (Client, error) { return fake, nil },
		Out:     &out,
		Err:     &errOut,
	}
	res, err := svc.Execute(context.Background(), args, map[string]string{EnvConnectionString: dsn})
	if err != nil {
		t.Fatalf("Execute returned engine error: %v", err)
	}
	return res, out.String(), errOut.String()
}

func TestExecuteMissingConnectionString(t *testing.T) {
	var out, errOut bytes.Buffer
	svc := &Service{Out: &out, Err: &errOut}
	res, err := svc.Execute(context.Background(), []string{"ping"}, map[string]string{})
	if err != nil {
		t.Fatalf("Execute returned engine error: %v", err)
	}
	if res.ExitCode != 1 || res.CredentialRejected {
		t.Errorf("result = %+v, want exit 1 without credential rejection", res)
	}
	if !strings.Contains(errOut.String(), "MONGODB_CONNECTION_STRING is not set") {
		t.Errorf("stderr = %q, want missing-env message", errOut.String())
	}
}

func TestPingEmitsOKAndDisconnects(t *testing.T) {
	fake := &fakeClient{}
	res, out, _ := run(t, fake, testDSN, "ping")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	if !strings.Contains(out, `"ok":true`) {
		t.Errorf("stdout = %q, want ok:true", out)
	}
	if !fake.disconnected {
		t.Error("client was not disconnected after execution")
	}
}

func TestFindUsesFlagDBFilterAndOptions(t *testing.T) {
	fake := &fakeClient{found: []bson.M{{"name": "ada"}}}
	res, out, _ := run(t, fake, testDSN,
		"find", "--db", "shop", "--collection", "users",
		"--filter", `{"age": {"$gt": 30}}`,
		"--sort", `{"age": -1, "name": 1}`,
		"--limit", "5", "--skip", "2")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	if fake.db != "shop" || fake.coll != "users" {
		t.Errorf("target = %s.%s, want shop.users", fake.db, fake.coll)
	}
	if fake.query.Limit != 5 || fake.query.Skip != 2 {
		t.Errorf("query = %+v, want limit 5 skip 2", fake.query)
	}
	if len(fake.query.Sort) != 2 || fake.query.Sort[0].Key != "age" {
		t.Errorf("sort = %+v, want ordered [age, name]", fake.query.Sort)
	}
	if len(fake.query.Filter) != 1 || fake.query.Filter[0].Key != "age" {
		t.Errorf("filter = %+v, want age condition", fake.query.Filter)
	}
	if !strings.Contains(out, `"name":"ada"`) || !strings.Contains(out, `"count":1`) {
		t.Errorf("stdout = %q, want documents + count", out)
	}
}

func TestDBDefaultsToDSNPath(t *testing.T) {
	fake := &fakeClient{collections: []string{"users"}}
	res, out, _ := run(t, fake,
		"mongodb://u:p@localhost:27017/mydb?authSource=admin",
		"collections", "list")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	if fake.db != "mydb" {
		t.Errorf("db = %q, want DSN default %q", fake.db, "mydb")
	}
	if !strings.Contains(out, `"collections":["users"]`) {
		t.Errorf("stdout = %q, want collections list", out)
	}
}

func TestNoDatabaseSelectedFails(t *testing.T) {
	fake := &fakeClient{}
	res, _, errOut := run(t, fake, testDSN, "collections", "list")
	if res.ExitCode != 1 || res.CredentialRejected {
		t.Errorf("result = %+v, want plain failure", res)
	}
	if !strings.Contains(errOut, "--db") {
		t.Errorf("stderr = %q, want --db guidance", errOut)
	}
}

func TestAuthenticationFailureRejectsCredential(t *testing.T) {
	fake := &fakeClient{err: mongo.CommandError{Code: 18, Message: "Authentication failed."}}
	res, _, errOut := run(t, fake, testDSN, "ping")
	if res.ExitCode != 1 || !res.CredentialRejected {
		t.Errorf("result = %+v, want exit 1 with credential rejection", res)
	}
	if !strings.Contains(errOut, "Authentication failed") {
		t.Errorf("stderr = %q, want provider message", errOut)
	}
}

func TestHandshakeAuthErrorRejectsCredential(t *testing.T) {
	fake := &fakeClient{err: errors.New(
		`connection() error occurred during connection handshake: auth error: sasl conversation error`)}
	res, _, _ := run(t, fake, testDSN, "ping")
	if res.ExitCode != 1 || !res.CredentialRejected {
		t.Errorf("result = %+v, want exit 1 with credential rejection", res)
	}
}

func TestUnauthorizedDoesNotRejectCredential(t *testing.T) {
	fake := &fakeClient{err: mongo.CommandError{Code: 13, Message: "not authorized on shop to execute command"}}
	res, _, _ := run(t, fake, testDSN, "find", "--db", "shop", "--collection", "users")
	if res.ExitCode != 1 || res.CredentialRejected {
		t.Errorf("result = %+v, want plain failure (permission != credential rejection)", res)
	}
}

func TestOrdinaryFailureDoesNotRejectCredential(t *testing.T) {
	fake := &fakeClient{err: errors.New("server selection timeout")}
	res, _, _ := run(t, fake, testDSN, "ping")
	if res.ExitCode != 1 || res.CredentialRejected {
		t.Errorf("result = %+v, want plain failure", res)
	}
}

func TestErrorOutputRedactsConnectionStringAndPassword(t *testing.T) {
	fake := &fakeClient{err: errors.New("cannot connect to " + testDSN + " (password s3cr3t-pw invalid)")}
	_, _, errOut := run(t, fake, testDSN, "ping")
	if strings.Contains(errOut, "s3cr3t-pw") {
		t.Errorf("stderr = %q, leaked the password", errOut)
	}
	if !strings.Contains(errOut, "[REDACTED]") {
		t.Errorf("stderr = %q, want [REDACTED] marker", errOut)
	}
}

func TestConnectErrorIsClassifiedAndRedacted(t *testing.T) {
	var out, errOut bytes.Buffer
	svc := &Service{
		Connect: func(context.Context, string) (Client, error) {
			return nil, errors.New("auth error: unable to authenticate with " + testDSN)
		},
		Out: &out,
		Err: &errOut,
	}
	res, err := svc.Execute(context.Background(), []string{"ping"},
		map[string]string{EnvConnectionString: testDSN})
	if err != nil {
		t.Fatalf("Execute returned engine error: %v", err)
	}
	if res.ExitCode != 1 || !res.CredentialRejected {
		t.Errorf("result = %+v, want exit 1 with credential rejection", res)
	}
	if strings.Contains(errOut.String(), "s3cr3t-pw") {
		t.Errorf("stderr = %q, leaked the password", errOut.String())
	}
}

func TestInsertUpdateDeleteOutputs(t *testing.T) {
	fake := &fakeClient{
		insertedIDs:  []any{"id-1", "id-2"},
		updateResult: &mongo.UpdateResult{MatchedCount: 3, ModifiedCount: 2},
		deleted:      4,
	}

	res, out, _ := run(t, fake, testDSN, "insert", "--db", "shop", "--collection", "users",
		"--doc", `{"name": "ada"}`, "--doc", `{"name": "bob"}`)
	if res.ExitCode != 0 || !strings.Contains(out, `"inserted_count":2`) {
		t.Errorf("insert result = %+v stdout %q, want inserted_count 2", res, out)
	}
	if len(fake.docs) != 2 {
		t.Errorf("insert docs = %d, want 2", len(fake.docs))
	}

	res, out, _ = run(t, fake, testDSN, "update", "--db", "shop", "--collection", "users",
		"--filter", `{"name": "ada"}`, "--update", `{"$set": {"vip": true}}`, "--many", "--upsert")
	if res.ExitCode != 0 || !strings.Contains(out, `"matched_count":3`) {
		t.Errorf("update result = %+v stdout %q, want matched_count 3", res, out)
	}
	if !fake.many || !fake.upsert {
		t.Errorf("update flags many=%v upsert=%v, want both true", fake.many, fake.upsert)
	}
	if len(fake.update) != 1 || fake.update[0].Key != "$set" {
		t.Errorf("update doc = %+v, want $set", fake.update)
	}

	res, out, _ = run(t, fake, testDSN, "delete", "--db", "shop", "--collection", "users",
		"--filter", `{"name": "bob"}`)
	if res.ExitCode != 0 || !strings.Contains(out, `"deleted_count":4`) {
		t.Errorf("delete result = %+v stdout %q, want deleted_count 4", res, out)
	}
	if fake.many {
		t.Error("delete defaulted to --many, want single-document delete")
	}
}

func TestAggregateParsesPipeline(t *testing.T) {
	fake := &fakeClient{found: []bson.M{{"total": 7}}}
	res, out, _ := run(t, fake, testDSN, "aggregate", "--db", "shop", "--collection", "orders",
		"--pipeline", `[{"$match": {"status": "paid"}}, {"$count": "total"}]`)
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	if len(fake.pipeline) != 2 {
		t.Fatalf("pipeline stages = %d, want 2", len(fake.pipeline))
	}
	if !strings.Contains(out, `"total":7`) {
		t.Errorf("stdout = %q, want aggregation result", out)
	}
}

func TestInvalidFilterJSONFails(t *testing.T) {
	fake := &fakeClient{}
	res, _, errOut := run(t, fake, testDSN, "find", "--db", "shop", "--collection", "users",
		"--filter", `{not json`)
	if res.ExitCode != 1 || res.CredentialRejected {
		t.Errorf("result = %+v, want plain failure", res)
	}
	if !strings.Contains(errOut, "invalid --filter") {
		t.Errorf("stderr = %q, want invalid --filter message", errOut)
	}
}

func TestDefaultDatabase(t *testing.T) {
	cases := []struct {
		dsn  string
		want string
	}{
		{"mongodb://u:p@localhost:27017/mydb", "mydb"},
		{"mongodb://u:p@localhost:27017/mydb?authSource=admin", "mydb"},
		{"mongodb+srv://u:p@cluster0.example.mongodb.net/?retryWrites=true", ""},
		{"mongodb+srv://u:p@cluster0.example.mongodb.net/appdb", "appdb"},
		{"mongodb://localhost:27017", ""},
	}
	for _, c := range cases {
		if got := defaultDatabase(c.dsn); got != c.want {
			t.Errorf("defaultDatabase(%q) = %q, want %q", c.dsn, got, c.want)
		}
	}
}
