package mongodb

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/heliohq/anycli/internal/tools/execution"
)

const testDSN = "mongodb+srv://appuser:s3cr3t-pw@cluster0.example.mongodb.net/?retryWrites=true"

// fixedMongoshFlags is the invariant argv prefix every invocation must carry.
var fixedMongoshFlags = []string{"--nodb", "--quiet", "--norc", "--json=relaxed"}

// fakeRun records the subprocess invocation and returns canned output.
type fakeRun struct {
	exitCode int
	stdout   string
	stderr   string

	called bool
	args   []string
	env    []string
	ctx    context.Context
}

func (f *fakeRun) run(ctx context.Context, args []string, env []string) (int, []byte, []byte, error) {
	f.called = true
	f.args = args
	f.env = env
	f.ctx = ctx
	return f.exitCode, []byte(f.stdout), []byte(f.stderr), nil
}

func run(t *testing.T, fake *fakeRun, dsn string, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	svc := &Service{Run: fake.run, Out: &out, Err: &errOut}
	res, err := svc.Execute(context.Background(), args, map[string]string{EnvConnectionString: dsn})
	if err != nil {
		t.Fatalf("Execute returned engine error: %v", err)
	}
	return res, out.String(), errOut.String()
}

func envValue(env []string, key string) (string, int) {
	value, count := "", 0
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			value = v
			count++
		}
	}
	return value, count
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

func TestMongoshArgsFixedFlagSet(t *testing.T) {
	args := mongoshArgs("db.users.find().toArray()")
	if len(args) != 6 {
		t.Fatalf("argv length = %d, want 6 (fixed flags + two --eval=)", len(args))
	}
	for i, want := range fixedMongoshFlags {
		if args[i] != want {
			t.Errorf("argv[%d] = %q, want %q", i, args[i], want)
		}
	}
	if args[4] != "--eval="+connectPrelude {
		t.Errorf("argv[4] = %q, want fused connect prelude", args[4])
	}
	if args[5] != "--eval=db.users.find().toArray()" {
		t.Errorf("argv[5] = %q, want fused script", args[5])
	}
}

// TestMongoshArgsScriptCannotOccupyFlagPosition pins the injection contract:
// the AI script is fused into a --eval= token, so even a script that IS a
// mongosh flag (--shell) never becomes a standalone argv token — there is no
// reachable path to extra mongosh flags.
func TestMongoshArgsScriptCannotOccupyFlagPosition(t *testing.T) {
	for _, script := range []string{"--shell", "--eval 1", "-f evil.js", "'; rm -rf /"} {
		args := mongoshArgs(script)
		for i, tok := range args {
			if i < len(fixedMongoshFlags) {
				continue
			}
			if !strings.HasPrefix(tok, "--eval=") {
				t.Errorf("script %q produced non-eval token %q at %d", script, tok, i)
			}
		}
		for _, tok := range args {
			if tok == "--shell" {
				t.Errorf("script %q leaked --shell as a standalone token", script)
			}
		}
	}
}

func TestArgvNeverContainsDSN(t *testing.T) {
	fake := &fakeRun{}
	run(t, fake, testDSN, "eval", "db.stats()")
	if !fake.called {
		t.Fatal("runner was not invoked")
	}
	for _, tok := range fake.args {
		if strings.Contains(tok, testDSN) || strings.Contains(tok, "s3cr3t-pw") {
			t.Errorf("argv token %q leaks the DSN", tok)
		}
	}
	if fake.args[4] != "--eval="+connectPrelude {
		t.Errorf("prelude token = %q, want env-variable-name literal only", fake.args[4])
	}
}

func TestEvalPassesScriptAndEnv(t *testing.T) {
	fake := &fakeRun{stdout: `{"ok": 1}`}
	res, out, _ := run(t, fake, testDSN, "eval", "db.getSiblingDB('shop').users.countDocuments()")
	if res.ExitCode != 0 || res.CredentialRejected {
		t.Fatalf("result = %+v, want clean success", res)
	}
	if !strings.Contains(out, `"ok": 1`) {
		t.Errorf("stdout = %q, want passthrough of mongosh output", out)
	}
	if got := fake.args[len(fake.args)-1]; got != "--eval=db.getSiblingDB('shop').users.countDocuments()" {
		t.Errorf("script token = %q", got)
	}

	dsn, n := envValue(fake.env, EnvConnectionString)
	if dsn != testDSN || n != 1 {
		t.Errorf("env %s = %q (count %d), want the DSN exactly once", EnvConnectionString, dsn, n)
	}
	home, n := envValue(fake.env, "HOME")
	if n != 1 || !strings.Contains(home, "anycli-mongosh-home-") {
		t.Errorf("env HOME = %q (count %d), want a scoped temp home", home, n)
	}
	if real := os.Getenv("HOME"); real != "" && home == real {
		t.Error("child HOME must not be the real home directory")
	}
}

func TestChildEnvDropsParentCollisions(t *testing.T) {
	t.Setenv(EnvConnectionString, "mongodb://parent-env-leak")
	t.Setenv("HOME", "/real/home")
	env := childEnv(testDSN, "/scoped/home")

	dsn, n := envValue(env, EnvConnectionString)
	if dsn != testDSN || n != 1 {
		t.Errorf("env %s = %q (count %d), want injected DSN exactly once", EnvConnectionString, dsn, n)
	}
	home, n := envValue(env, "HOME")
	if home != "/scoped/home" || n != 1 {
		t.Errorf("env HOME = %q (count %d), want scoped home exactly once", home, n)
	}
}

func TestPingRunsPingScript(t *testing.T) {
	fake := &fakeRun{stdout: `{"ok": 1}`}
	res, out, _ := run(t, fake, testDSN, "ping")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	if got := fake.args[len(fake.args)-1]; got != "--eval="+pingScript {
		t.Errorf("ping script token = %q, want %q", got, "--eval="+pingScript)
	}
	if !strings.Contains(out, `"ok": 1`) {
		t.Errorf("stdout = %q, want ping result", out)
	}
}

func TestUnknownFlagsAreRejectedBeforeSpawning(t *testing.T) {
	fake := &fakeRun{}
	res, _, errOut := run(t, fake, testDSN, "eval", "db.stats()", "--shell")
	if res.ExitCode != 1 || res.CredentialRejected {
		t.Errorf("result = %+v, want plain failure", res)
	}
	if fake.called {
		t.Error("runner was invoked despite an unknown flag")
	}
	if !strings.Contains(errOut, "--shell") {
		t.Errorf("stderr = %q, want unknown-flag mention", errOut)
	}
}

func TestExtraPositionalArgsRejected(t *testing.T) {
	fake := &fakeRun{}
	res, _, _ := run(t, fake, testDSN, "eval", "db.stats()", "extra-script")
	if res.ExitCode != 1 || fake.called {
		t.Errorf("result = %+v called=%t, want rejection before spawning", res, fake.called)
	}
}

// TestAuthenticationFailedRejectsCredential pins the production error
// channel: mongosh --json prints a thrown error as a relaxed-EJSON object on
// STDOUT (stderr stays empty), so auth classification must read stdout. A
// stderr-text case remains as the belt for output bypassing the JSON reporter.
func TestAuthenticationFailedRejectsCredential(t *testing.T) {
	cases := []struct {
		name   string
		stdout string
		stderr string
	}{
		{
			name: "server auth failure (stdout JSON, codeName)",
			stdout: `{
  "message": "Authentication failed.",
  "stack": "MongoServerError: Authentication failed.\n    at ...",
  "name": "MongoServerError",
  "ok": 0,
  "code": 18,
  "codeName": "AuthenticationFailed"
}`,
		},
		{
			name: "atlas bad auth (stdout JSON, no codeName)",
			stdout: `{
  "message": "bad auth : authentication failed",
  "stack": "MongoServerSelectionError: bad auth : authentication failed\n    at ...",
  "name": "MongoServerSelectionError"
}`,
		},
		{
			name:   "handshake auth error on stderr (non-JSON belt)",
			stderr: "connection() error occurred during connection handshake: auth error: sasl conversation error",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := &fakeRun{exitCode: 1, stdout: c.stdout, stderr: c.stderr}
			res, out, errOut := run(t, fake, testDSN, "ping")
			if res.ExitCode != 1 || !res.CredentialRejected {
				t.Errorf("result = %+v, want exit 1 with credential rejection", res)
			}
			if !strings.Contains(strings.ToLower(out+errOut), "auth") {
				t.Errorf("output = %q %q, want provider message passthrough", out, errOut)
			}
		})
	}
}

// TestUnauthorizedDoesNotRejectCredential pins the driver-era distinction:
// Unauthorized (permission) failures must NOT invalidate the credential. The
// server's codeName on the stdout JSON error object is authoritative.
func TestUnauthorizedDoesNotRejectCredential(t *testing.T) {
	fake := &fakeRun{exitCode: 1, stdout: `{
  "message": "not authorized on shop to execute command { find: \"users\" }",
  "name": "MongoServerError",
  "ok": 0,
  "code": 13,
  "codeName": "Unauthorized"
}`}
	res, _, _ := run(t, fake, testDSN, "eval", "db.users.find()")
	if res.ExitCode != 1 || res.CredentialRejected {
		t.Errorf("result = %+v, want plain failure (permission != credential rejection)", res)
	}
}

func TestOrdinaryFailureDoesNotRejectCredential(t *testing.T) {
	fake := &fakeRun{exitCode: 1, stdout: `{
  "message": "connect ECONNREFUSED 127.0.0.1:27017",
  "stack": "MongoNetworkError: connect ECONNREFUSED 127.0.0.1:27017\n    at ...",
  "name": "MongoNetworkError"
}`}
	res, _, _ := run(t, fake, testDSN, "ping")
	if res.ExitCode != 1 || res.CredentialRejected {
		t.Errorf("result = %+v, want plain failure", res)
	}
}

func TestLastJSONErrorObject(t *testing.T) {
	cases := []struct {
		name      string
		stdout    string
		wantFound bool
		wantName  string
	}{
		{"empty", "", false, ""},
		{"non-JSON text", "some plain output\n", false, ""},
		{"success result without error shape", `{"ok": 1}`, false, ""},
		{"single error object", `{"message": "boom", "name": "Error"}`, true, "Error"},
		{
			"error object after earlier JSON value",
			`{"ok": 1}` + "\n" + `{"message": "Authentication failed.", "name": "MongoServerError", "codeName": "AuthenticationFailed"}`,
			true, "MongoServerError",
		},
		{"trailing garbage stops the scan", `{"message": "boom", "name": "Error"}` + "\nnot-json", true, "Error"},
		{
			"user document with name/message fields is not an error object",
			`{"name": "job-import", "message": "authentication failed for worker 3", "ts": 1}`,
			false, "",
		},
		{
			"non-Error name with codeName still counts (server error shape)",
			`{"name": "MongoServerFault", "message": "x", "codeName": "Unauthorized"}`,
			true, "MongoServerFault",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, found := lastJSONErrorObject([]byte(c.stdout))
			if found != c.wantFound || got.Name != c.wantName {
				t.Errorf("lastJSONErrorObject = (%+v, %t), want name %q found %t", got, found, c.wantName, c.wantFound)
			}
		})
	}
}

// TestUserDocumentEchoDoesNotRejectCredential pins the isErrorShape guard:
// a printed user document that merely has name/message fields (e.g. a row from
// an error-log collection whose message says "authentication failed") followed
// by a non-zero exit must not invalidate the credential.
func TestUserDocumentEchoDoesNotRejectCredential(t *testing.T) {
	fake := &fakeRun{exitCode: 1, stdout: `{
  "name": "job-import",
  "message": "authentication failed for upstream worker",
  "level": "error"
}`}
	res, _, _ := run(t, fake, testDSN, "eval", "printjson(db.errorlog.findOne()); quit(1)")
	if res.ExitCode != 1 || res.CredentialRejected {
		t.Errorf("result = %+v, want plain failure (user document is not an auth report)", res)
	}
}

func TestExitCodeIsPropagated(t *testing.T) {
	fake := &fakeRun{exitCode: 3, stderr: "some scripted failure"}
	res, _, _ := run(t, fake, testDSN, "eval", "quit(3)")
	if res.ExitCode != 3 {
		t.Errorf("exit = %d, want mongosh's own 3", res.ExitCode)
	}
}

func TestOutputRedactsConnectionStringAndPassword(t *testing.T) {
	fake := &fakeRun{
		exitCode: 1,
		stdout:   "connected to " + testDSN,
		stderr:   "cannot connect to " + testDSN + " (password s3cr3t-pw invalid)",
	}
	_, out, errOut := run(t, fake, testDSN, "ping")
	for name, stream := range map[string]string{"stdout": out, "stderr": errOut} {
		if strings.Contains(stream, "s3cr3t-pw") {
			t.Errorf("%s = %q, leaked the password", name, stream)
		}
		if !strings.Contains(stream, "[REDACTED]") {
			t.Errorf("%s = %q, want [REDACTED] marker", name, stream)
		}
	}
}

func TestRunnerContextCarriesDeadline(t *testing.T) {
	fake := &fakeRun{}
	run(t, fake, testDSN, "ping")
	if fake.ctx == nil {
		t.Fatal("runner did not receive a context")
	}
	if _, ok := fake.ctx.Deadline(); !ok {
		t.Error("runner context has no deadline; a hung mongosh would hang forever")
	}
}

func TestTimeoutFlagAborts(t *testing.T) {
	blocking := func(ctx context.Context, args []string, env []string) (int, []byte, []byte, error) {
		<-ctx.Done()
		return -1, nil, nil, ctx.Err()
	}
	var out, errOut bytes.Buffer
	svc := &Service{Run: blocking, Out: &out, Err: &errOut}
	start := time.Now()
	res, err := svc.Execute(context.Background(), []string{"ping", "--timeout", "50ms"},
		map[string]string{EnvConnectionString: testDSN})
	if err != nil {
		t.Fatalf("Execute returned engine error: %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("timeout did not abort the invocation")
	}
	if res.ExitCode != 1 || res.CredentialRejected {
		t.Errorf("result = %+v, want plain exit 1", res)
	}
	if !strings.Contains(errOut.String(), "timed out") {
		t.Errorf("stderr = %q, want timeout message", errOut.String())
	}
}

func TestHelpMentionsWrappedPinAndLazyDownload(t *testing.T) {
	fake := &fakeRun{}
	res, out, _ := run(t, fake, testDSN, "--help")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0 for --help", res.ExitCode)
	}
	if fake.called {
		t.Error("help must not spawn mongosh")
	}
	for _, want := range []string{
		"wraps mongosh 2.9.2",
		"downloads.mongodb.com",
		"first invocation downloads",
		// PATH is only used while no pinned install exists — the help must not
		// claim unconditional PATH precedence (pin level ① resolves first).
		"only while no pinned",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("help output missing %q\n%s", want, out)
		}
	}
}

func TestRedactSecret(t *testing.T) {
	in := "failed for " + testDSN + " pw=s3cr3t-pw"
	got := redactSecret(in, testDSN)
	if strings.Contains(got, "s3cr3t-pw") || strings.Contains(got, testDSN) {
		t.Errorf("redactSecret = %q, secrets survived", got)
	}
}

// TestRedactSecretPercentEncodedPassword pins that both the decoded password
// (as url.Parse reports it) and the percent-encoded literal spelled inside the
// DSN are redacted — output may echo either form.
func TestRedactSecretPercentEncodedPassword(t *testing.T) {
	dsn := "mongodb://appuser:p%40ss%2Fw0rd@db.example.com:27017/shop?authSource=admin"
	in := "bad literal p%40ss%2Fw0rd and decoded p@ss/w0rd in output"
	got := redactSecret(in, dsn)
	for _, leak := range []string{"p%40ss%2Fw0rd", "p@ss/w0rd"} {
		if strings.Contains(got, leak) {
			t.Errorf("redactSecret = %q, leaked %q", got, leak)
		}
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Errorf("redactSecret = %q, want [REDACTED] marker", got)
	}
}

func TestLiteralDSNPassword(t *testing.T) {
	cases := []struct {
		dsn  string
		want string
	}{
		{"mongodb://user:p%40ss@host:27017/db", "p%40ss"},
		{"mongodb+srv://user:plain@cluster0.example.mongodb.net/?retryWrites=true", "plain"},
		{"mongodb://user@host/db", ""},
		{"mongodb://host:27017/db", ""},
	}
	for _, c := range cases {
		if got := literalDSNPassword(c.dsn); got != c.want {
			t.Errorf("literalDSNPassword(%q) = %q, want %q", c.dsn, got, c.want)
		}
	}
}
