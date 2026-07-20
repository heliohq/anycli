// Package mongodb is the built-in MongoDB service: a non-interactive cobra
// tree over database/collection operations, connected through a host-resolved
// connection string. It is the first non-HTTP service tool — auth failures
// come from the driver as command errors, not HTTP status codes.
package mongodb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// EnvConnectionString is the env var the credential binding injects
// (definitions/tools/mongodb.json). The resolved connection_string is a
// standard MongoDB DSN (mongodb:// or mongodb+srv://).
const EnvConnectionString = "MONGODB_CONNECTION_STRING"

// authenticationFailedCode is MongoDB's AuthenticationFailed server error
// code: the provider explicitly rejected the credential. Code 13
// (Unauthorized) is a permission failure and must NOT reject the credential.
const authenticationFailedCode = 18

// Service implements the built-in MongoDB tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// Connect overrides the driver constructor; nil uses the real mongo
	// driver. Tests inject a fake Client.
	Connect func(ctx context.Context, uri string) (Client, error)
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one mongodb subcommand with the resolved connection string in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	dsn := env[EnvConnectionString]
	if dsn == "" {
		fmt.Fprintln(s.stderr(), "MONGODB_CONNECTION_STRING is not set")
		return execution.Result{ExitCode: 1}, nil
	}
	root := s.newRoot(dsn)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(s.stderr(), redactSecret(err.Error(), dsn))
		return execution.Failure(err), nil
	}
	return execution.Result{}, nil
}

func (s *Service) stdout() io.Writer {
	if s.Out != nil {
		return s.Out
	}
	return os.Stdout
}

func (s *Service) stderr() io.Writer {
	if s.Err != nil {
		return s.Err
	}
	return os.Stderr
}

// withClient connects, runs fn, disconnects, and classifies provider auth
// rejections so the engine can invalidate the credential.
func (s *Service) withClient(ctx context.Context, dsn string, fn func(Client) error) error {
	connect := s.Connect
	if connect == nil {
		connect = driverConnect
	}
	c, err := connect(ctx, dsn)
	if err != nil {
		return classify(err)
	}
	defer func() { _ = c.Disconnect(context.Background()) }()
	return classify(fn(c))
}

// classify wraps explicit provider credential rejections with
// execution.RejectCredential; ordinary failures pass through unchanged.
func classify(err error) error {
	if err == nil {
		return nil
	}
	if isAuthError(err) {
		return execution.RejectCredential(err)
	}
	return err
}

func isAuthError(err error) bool {
	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) {
		return cmdErr.Code == authenticationFailedCode
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "authentication failed") || strings.Contains(msg, "auth error")
}

// emit writes one relaxed extended-JSON document to stdout.
func (s *Service) emit(doc any) error {
	b, err := bson.MarshalExtJSON(doc, false, false)
	if err != nil {
		return fmt.Errorf("mongodb: encode output: %w", err)
	}
	_, err = s.stdout().Write(append(b, '\n'))
	return err
}

// resolveDB picks the target database: the per-invocation --db flag wins; the
// connection string's path component is the optional default. The Atlas
// "Connect your application" DSN has no path — --db is required then.
func resolveDB(flagDB, dsn string) (string, error) {
	if flagDB != "" {
		return flagDB, nil
	}
	if db := defaultDatabase(dsn); db != "" {
		return db, nil
	}
	return "", errors.New("no database selected: pass --db (the connection string has no default database)")
}

// defaultDatabase extracts the auth-database path from a MongoDB DSN. An
// unparsable DSN yields no default; the driver reports the real error.
func defaultDatabase(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(u.Path, "/")
}

// redactSecret removes the connection string (and its password component)
// from provider error text before it reaches stderr.
func redactSecret(value, dsn string) string {
	if dsn == "" {
		return value
	}
	value = strings.ReplaceAll(value, dsn, "[REDACTED]")
	if u, err := url.Parse(dsn); err == nil && u.User != nil {
		if pw, ok := u.User.Password(); ok && pw != "" {
			value = strings.ReplaceAll(value, pw, "[REDACTED]")
		}
	}
	return value
}
