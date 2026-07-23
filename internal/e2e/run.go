package e2e

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	anycli "github.com/heliohq/anycli"
)

var (
	engineOnce sync.Once
	engine     *anycli.Engine
	engineErr  error

	// stdoutMu serializes stdout capture: built-in services print their
	// JSON to os.Stdout, so concurrent RunTool calls in one process would
	// interleave. Closed-loop e2e chains are sequential anyway.
	stdoutMu sync.Mutex

	prefixOnce sync.Once
	prefix     string
)

// Prefix returns the run-scoped test-data prefix "anycli-e2e-<runid>-"
// (design 008 D4): GITHUB_RUN_ID in CI, a timestamp locally. All data an
// e2e test creates must carry it so interrupted-run leftovers are
// identifiable by the nightly sweep.
func Prefix() string {
	prefixOnce.Do(func() {
		id := os.Getenv("GITHUB_RUN_ID")
		if id == "" {
			id = fmt.Sprintf("%d", time.Now().Unix())
		}
		prefix = "anycli-e2e-" + id + "-"
	})
	return prefix
}

// RunTool executes one tool invocation through the real engine with the e2e
// resolver and returns its captured stdout and exit code. The short form of
// RunToolWithStderr for tests that don't inspect error text.
func RunTool(t *testing.T, tool, account string, args ...string) (string, int) {
	t.Helper()
	out, _, exit := RunToolWithStderr(t, tool, account, args...)
	return out, exit
}

// RunToolWithStderr executes one tool invocation through the real engine
// with the e2e resolver and returns its captured stdout, stderr, and exit
// code. Services print API error details to stderr, so tests that need to
// distinguish failure kinds (e.g. bitly's 402 UPGRADE_REQUIRED plan gating)
// use this form and match on stderr instead of tolerating every nonzero
// exit.
//
// Skip semantics (design 008): missing e2e configuration or a not-connected
// tool skips the test (t.Skip) with an "E2E-PENDING" marker the workflow
// greps into the job summary. Engine-level errors fail the test. A nonzero
// exit code is returned, not fatal — closed-loop tests assert on it (e.g.
// "get after delete must fail").
func RunToolWithStderr(t *testing.T, tool, account string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()

	resolver, err := NewResolver()
	if err != nil {
		if credentialFromEnv(account) != nil {
			// Local override present: run with a resolver that only
			// serves env credentials (gateway config not required).
			resolver = &Resolver{}
		} else {
			t.Skipf("E2E-PENDING tool=%s: %v", tool, err)
		}
	}

	engineOnce.Do(func() {
		engine, engineErr = anycli.New(anycli.Config{})
	})
	if engineErr != nil {
		t.Fatalf("anycli.New: %v", engineErr)
	}

	// stdoutMu is held for the whole capture. If t.Skipf/t.Fatalf below
	// unwind via runtime.Goexit (not a normal return), this deferred
	// Unlock still runs during the goroutine's exit — but only because
	// RunTool executes on the test goroutine. Calling RunTool from any
	// other goroutine would break that invariant and could deadlock or
	// unlock out of turn.
	stdoutMu.Lock()
	defer stdoutMu.Unlock()

	var exit int
	var execErr error
	out, errOut, capErr := captureOutput(func() error {
		exit, execErr = engine.ExecuteWith(context.Background(), anycli.Tool(tool), args, resolver,
			anycli.ExecOptions{Account: account})
		return nil
	})
	if capErr != nil {
		t.Fatalf("capture output: %v", capErr)
	}
	if execErr != nil {
		if IsNotConnected(execErr) {
			t.Skipf("E2E-PENDING tool=%s account=%q: %v", tool, account, execErr)
		}
		// API-level failures also surface as an error next to a nonzero
		// exit; log it and let the caller assert on the exit code.
		t.Logf("tool %s exit=%d err: %v", tool, exit, execErr)
	}
	return out, errOut, exit
}

// captureOutput redirects os.Stdout and os.Stderr around fn and returns
// what fn printed to each.
//
// Restoring the streams and closing the write ends happen in a defer so
// they still run if fn panics: otherwise the globals would stay pointed at
// orphaned pipes and the drain goroutines below would block forever on
// io.ReadAll.
func captureOutput(fn func() error) (out, errOut string, err error) {
	oldOut, oldErr := os.Stdout, os.Stderr
	rOut, wOut, perr := os.Pipe()
	if perr != nil {
		return "", "", perr
	}
	rErr, wErr, perr := os.Pipe()
	if perr != nil {
		rOut.Close()
		wOut.Close()
		return "", "", perr
	}
	os.Stdout, os.Stderr = wOut, wErr

	drain := func(r *os.File) chan string {
		done := make(chan string, 1)
		go func() {
			b, _ := io.ReadAll(r)
			r.Close()
			done <- string(b)
		}()
		return done
	}
	doneOut, doneErr := drain(rOut), drain(rErr)

	defer func() {
		os.Stdout, os.Stderr = oldOut, oldErr
		wOut.Close()
		wErr.Close()
		out = <-doneOut
		errOut = <-doneErr
	}()

	return "", "", fn()
}

// osStdoutWriteString / osStderrWriteString exist for the capture unit
// test: they write through the (possibly redirected) globals at call time.
func osStdoutWriteString(s string) (int, error) {
	return os.Stdout.WriteString(s)
}

func osStderrWriteString(s string) (int, error) {
	return os.Stderr.WriteString(s)
}
