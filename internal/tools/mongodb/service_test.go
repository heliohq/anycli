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

func TestAuthenticationFailedStderrRejectsCredential(t *testing.T) {
	cases := []struct {
		name   string
		stderr string
	}{
		{"server auth failure", "MongoServerError: Authentication failed."},
		{"atlas bad auth", "MongoServerSelectionError: bad auth : authentication failed"},
		{"handshake auth error", "connection() error occurred during connection handshake: auth error: sasl conversation error"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := &fakeRun{exitCode: 1, stderr: c.stderr}
			res, _, errOut := run(t, fake, testDSN, "ping")
			if res.ExitCode != 1 || !res.CredentialRejected {
				t.Errorf("result = %+v, want exit 1 with credential rejection", res)
			}
			if !strings.Contains(strings.ToLower(errOut), "auth") {
				t.Errorf("stderr = %q, want provider message passthrough", errOut)
			}
		})
	}
}

// TestUnauthorizedDoesNotRejectCredential pins the driver-era distinction:
// Unauthorized (permission) failures must NOT invalidate the credential.
func TestUnauthorizedDoesNotRejectCredential(t *testing.T) {
	fake := &fakeRun{exitCode: 1, stderr: "MongoServerError: not authorized on shop to execute command { find: \"users\" }"}
	res, _, _ := run(t, fake, testDSN, "eval", "db.users.find()")
	if res.ExitCode != 1 || res.CredentialRejected {
		t.Errorf("result = %+v, want plain failure (permission != credential rejection)", res)
	}
}

func TestOrdinaryFailureDoesNotRejectCredential(t *testing.T) {
	fake := &fakeRun{exitCode: 1, stderr: "MongoNetworkError: connect ECONNREFUSED 127.0.0.1:27017"}
	res, _, _ := run(t, fake, testDSN, "ping")
	if res.ExitCode != 1 || res.CredentialRejected {
		t.Errorf("result = %+v, want plain failure", res)
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
	for _, want := range []string{"wraps mongosh 2.9.2", "downloads.mongodb.com", "first invocation downloads"} {
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
