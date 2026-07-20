// Package mongodb is the built-in MongoDB tool: a thin wrapper around the
// official MongoDB Shell (mongosh). It exposes exactly two arms — eval (run a
// mongosh JavaScript snippet) and ping — instead of a hand-rolled verb subset:
// models are fluent in mongosh JS, and any verb list would forever chase it.
//
// The subprocess is spawned with an execve-style argv (no shell interpolation)
// over a fixed flag set; the connection string travels only through the child
// environment (MONGODB_CONNECTION_STRING), never through argv, so `ps` never
// shows it. mongosh itself is lazily installed from the official download
// host on first use (see internal/exec/binresolve).
package mongodb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/heliohq/anycli/definitions"
	"github.com/heliohq/anycli/internal/config"
	"github.com/heliohq/anycli/internal/exec/binresolve"
	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// EnvConnectionString is the env var the credential binding injects
// (definitions/tools/mongodb.json). The resolved connection_string is a
// standard MongoDB DSN (mongodb:// or mongodb+srv://).
const EnvConnectionString = "MONGODB_CONNECTION_STRING"

// connectPrelude is the first --eval: it dials the connection string from the
// child environment, then deletes the variable so the AI script running in the
// same process cannot read the DSN back via process.env. The argv contains
// only this variable-name literal — the DSN value itself never appears on the
// command line.
//
// The delete closes the most obvious read channel only (residuals remain:
// /proc/self/environ on Linux, the Mongo object's own URI). Likewise
// redactSecret below guards against ACCIDENTAL echo of the DSN, not deliberate
// exfiltration by the script — the real boundary is a database-side read-only
// role (design 313 Future Work).
const connectPrelude = "db = connect(process.env." + EnvConnectionString + "); " +
	"delete process.env." + EnvConnectionString

// pingScript is the second --eval for the ping arm.
const pingScript = "db.runCommand({ ping: 1 })"

// defaultTimeout bounds one mongosh invocation when the engine context has no
// deadline of its own.
const defaultTimeout = 2 * time.Minute

// Runner executes one mongosh subprocess: args is the full mongosh argv (after
// the binary), env is the complete child environment. Tests inject a fake; the
// zero-value Service resolves the pinned mongosh (lazy-installing on first
// use) and runs it with no TTY.
type Runner func(ctx context.Context, args []string, env []string) (exitCode int, stdout, stderr []byte, err error)

// Service implements the built-in MongoDB tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// Run overrides subprocess execution; nil resolves and executes the real
	// mongosh binary.
	Run Runner
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
	inv := &invocation{}
	root := s.newRoot(dsn, inv)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(s.stderr(), redactSecret(err.Error(), dsn))
		exitCode := inv.exitCode
		if exitCode <= 0 {
			exitCode = 1
		}
		return execution.Result{
			ExitCode:           exitCode,
			CredentialRejected: execution.IsCredentialRejected(err),
		}, nil
	}
	return execution.Result{}, nil
}

// invocation carries the mongosh exit code from the command RunE back to
// Execute so the service propagates it instead of flattening to 1.
type invocation struct {
	exitCode int
}

func (s *Service) newRoot(dsn string, inv *invocation) *cobra.Command {
	pin := pinnedMongoshVersion()
	root := &cobra.Command{
		Use:   "mongodb",
		Short: fmt.Sprintf("MongoDB via the official MongoDB Shell (wraps mongosh %s)", pin),
		Long: fmt.Sprintf(`MongoDB via the official MongoDB Shell — wraps mongosh %[1]s.

The first invocation downloads mongosh %[1]s from downloads.mongodb.com
(sha256-verified) if it is not already installed; later invocations reuse it.
A mongosh already on PATH takes precedence and is used as-is (no download).

Two commands:
  eval <script>   Run a mongosh JavaScript snippet against the connected
                  deployment. "db" is pre-connected from the configured
                  connection string; use standard mongosh JS, e.g.
                    eval 'db.getSiblingDB("shop").users.find({age: {$gt: 30}}).toArray()'
                  Output is relaxed extended JSON (mongosh --json=relaxed).
                  Scripts starting with "-" need a "--" separator: eval -- '<script>'.
  ping            Verify connectivity and authentication.

There is no --db flag: select databases in the script (db.getSiblingDB(...)).
mongosh flags are fixed and not passed through.`, pin),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	timeout := root.PersistentFlags().Duration("timeout", defaultTimeout, "mongosh execution timeout")

	evalCmd := &cobra.Command{
		Use:   "eval <script>",
		Short: "Run a mongosh JavaScript snippet",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.runMongosh(cmd.Context(), args[0], dsn, *timeout, inv)
		},
	}
	pingCmd := &cobra.Command{
		Use:   "ping",
		Short: "Verify connectivity and authentication",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.runMongosh(cmd.Context(), pingScript, dsn, *timeout, inv)
		},
	}
	root.AddCommand(evalCmd, pingCmd)
	return root
}

// mongoshArgs assembles the full execve argv (after the binary) for one
// script. The flag set is fixed; the prelude and the script are fused into
// --eval= tokens so script content can never occupy a flag position (there is
// no reachable path to --shell or any other mongosh flag).
func mongoshArgs(script string) []string {
	return []string{
		"--nodb",
		"--quiet",
		"--norc",
		"--json=relaxed",
		"--eval=" + connectPrelude,
		"--eval=" + script,
	}
}

// runMongosh executes one mongosh invocation: fixed argv, credential and
// scoped HOME in the child env, redacted output, auth classification, and a
// context deadline.
func (s *Service) runMongosh(ctx context.Context, script, dsn string, timeout time.Duration, inv *invocation) error {
	run := s.Run
	if run == nil {
		// Resolve — and on first call lazy-install — mongosh BEFORE the
		// execution timeout starts: a ~55MB first-call download must not eat
		// the query budget or surface as a misleading "query timed out".
		binary, err := s.resolveMongoshBinary(ctx)
		if err != nil {
			return err
		}
		run = mongoshRunner(binary)
	}

	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	scopedHome, err := os.MkdirTemp("", "anycli-mongosh-home-")
	if err != nil {
		return fmt.Errorf("create scoped mongosh home: %w", err)
	}
	defer os.RemoveAll(scopedHome)

	exitCode, stdout, stderrOut, runErr := run(ctx, mongoshArgs(script), childEnv(dsn, scopedHome))
	inv.exitCode = exitCode

	if len(stdout) > 0 {
		s.stdout().Write([]byte(redactSecret(string(stdout), dsn)))
	}
	if len(stderrOut) > 0 {
		s.stderr().Write([]byte(redactSecret(string(stderrOut), dsn)))
	}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("mongosh timed out after %s", timeout)
	}
	if runErr != nil {
		return classifyFailure(runErr, stdout, stderrOut)
	}
	if exitCode != 0 {
		return classifyFailure(fmt.Errorf("mongosh exited with code %d", exitCode), stdout, stderrOut)
	}
	return nil
}

// childEnv builds the subprocess environment: the parent env with HOME (and
// USERPROFILE) redirected to a scoped temp dir and the connection string
// injected. The DSN exists only here — never in argv.
func childEnv(dsn, scopedHome string) []string {
	parent := os.Environ()
	env := make([]string, 0, len(parent)+3)
	for _, kv := range parent {
		key, _, _ := strings.Cut(kv, "=")
		switch key {
		case "HOME", "USERPROFILE", EnvConnectionString:
			continue
		}
		env = append(env, kv)
	}
	return append(env,
		"HOME="+scopedHome,
		"USERPROFILE="+scopedHome,
		EnvConnectionString+"="+dsn,
	)
}

// resolveMongoshBinary resolves the pinned mongosh (lazy-installing from the
// official source on first use). Install progress notices go to the service's
// stderr stream.
func (s *Service) resolveMongoshBinary(ctx context.Context) (string, error) {
	def, err := definitions.LoadBundled("mongodb")
	if err != nil {
		return "", err
	}
	binary, err := binresolve.Resolve(ctx, def.Name, def.Binary, def.Source, binresolve.Options{
		SkipPATHDir: config.BinDir(),
		Notice:      s.stderr(),
	})
	if err != nil {
		return "", fmt.Errorf("resolve mongosh: %w", err)
	}
	return binary, nil
}

// mongoshRunner is the production Runner factory: it runs the resolved binary
// with stdin closed (no TTY).
func mongoshRunner(binary string) Runner {
	return func(ctx context.Context, args []string, env []string) (int, []byte, []byte, error) {
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Env = env
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		runErr := cmd.Run()

		exitCode := 1
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		}
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			// Non-zero exit is reported through exitCode + the output
			// streams, not as an exec-level error.
			runErr = nil
		}
		return exitCode, stdout.Bytes(), stderr.Bytes(), runErr
	}
}

// classifyFailure wraps explicit provider credential rejections with
// execution.RejectCredential. In --json mode mongosh reports a thrown error —
// including auth failures — as a relaxed-EJSON object on STDOUT with an empty
// stderr, so classification parses that object and keys off codeName / name /
// message; the stderr text check remains only as a belt for output that
// bypasses the JSON reporter. Permission errors (codeName Unauthorized /
// "not authorized …") and ordinary failures pass through unchanged — same
// semantics as the retired driver implementation.
func classifyFailure(err error, stdout, stderr []byte) error {
	if errObj, ok := lastJSONErrorObject(stdout); ok {
		if errObj.rejectsCredential() {
			return execution.RejectCredential(err)
		}
		// A parsed error object is authoritative: an explicit Unauthorized /
		// network error must not be re-classified from stray stderr text.
		return err
	}
	if isAuthText(string(stderr)) {
		return execution.RejectCredential(err)
	}
	return err
}

// mongoshError is the relaxed-EJSON error object mongosh --json prints on
// stdout for a thrown error ({"message": ..., "name": ..., "codeName": ...}).
type mongoshError struct {
	Message  string `json:"message"`
	Name     string `json:"name"`
	CodeName string `json:"codeName"`
}

// rejectsCredential reports whether the error is an explicit authentication
// rejection. The server codeName is authoritative when present
// (AuthenticationFailed rejects; Unauthorized is a permission error and never
// does); errors without a codeName (e.g. Atlas "bad auth : authentication
// failed" via MongoServerSelectionError) fall back to message text.
func (e mongoshError) rejectsCredential() bool {
	switch e.CodeName {
	case "AuthenticationFailed":
		return true
	case "Unauthorized":
		return false
	}
	return isAuthText(e.Message)
}

// lastJSONErrorObject decodes successive JSON values from stdout and returns
// the last one shaped like a mongosh error object. mongosh prints only the
// final eval's result, so on failure the error object is normally the whole
// stdout; scanning all values tolerates any preceding output.
func lastJSONErrorObject(stdout []byte) (mongoshError, bool) {
	dec := json.NewDecoder(bytes.NewReader(stdout))
	var last mongoshError
	found := false
	for {
		var candidate mongoshError
		if err := dec.Decode(&candidate); err != nil {
			break
		}
		if candidate.Name != "" && candidate.Message != "" {
			last, found = candidate, true
		}
	}
	return last, found
}

func isAuthText(text string) bool {
	msg := strings.ToLower(text)
	return strings.Contains(msg, "authentication failed") || strings.Contains(msg, "auth error")
}

// pinnedMongoshVersion reads the mongosh pin from the embedded definition once.
var pinnedMongoshVersion = sync.OnceValue(func() string {
	def, err := definitions.LoadBundled("mongodb")
	if err != nil || def.Source == nil {
		return "(unpinned)"
	}
	return def.Source.Version
})

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

// redactSecret removes the connection string (and its password component)
// from subprocess output and error text before it reaches the caller.
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
