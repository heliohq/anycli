package exec

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/heliohq/anycli/definitions"
	"github.com/heliohq/anycli/internal/config"
	"github.com/heliohq/anycli/internal/credential"
	"github.com/heliohq/anycli/internal/dryrun"
	"github.com/heliohq/anycli/internal/exec/binresolve"
	"github.com/heliohq/anycli/internal/middleware"
	"github.com/heliohq/anycli/internal/registry"
	"github.com/heliohq/anycli/internal/toolhelp"
	"github.com/heliohq/anycli/internal/tools"
)

// loadDefinition loads a tool definition by name. It is a package variable so
// tests can inject synthetic definitions; production always loads from the
// embedded definition set.
var loadDefinition = definitions.LoadBundled

// helpOut overrides the sink for the flattened service-tool help face. It is a
// package variable so tests can capture the rendering; nil — the production
// value — means "the process's stdout".
//
// Deliberately NOT `= os.Stdout`: that binds the *os.File value at package init
// and stops following a caller that swaps the global. internal/e2e's
// captureOutput does exactly that (os.Pipe + `os.Stdout = wOut` around
// engine.ExecuteWith), so an init-time snapshot would write the face to the
// original fd — an e2e asserting on it would compare against "" while the text
// leaked to the CI log. Every other output in this file reads the global at
// call time; helpWriter keeps this one consistent with them.
var helpOut io.Writer

// helpWriter resolves the help sink at CALL time. Any host — internal/e2e, a
// heliox command, a test — may swap os.Stdout around Execute.
func helpWriter() io.Writer {
	if helpOut != nil {
		return helpOut
	}
	return os.Stdout
}

// Engine runs tools through the full credential + middleware pipeline. It holds
// the consumer-supplied credential cache; tool definitions come from the
// internal embedded set, not from the engine's configuration.
type Engine struct {
	cache credential.Cache
}

// NewEngine constructs an Engine with the given credential cache. cache must be
// non-nil; the public constructor installs the in-memory default when the
// consumer supplies none.
func NewEngine(cache credential.Cache) (*Engine, error) {
	if cache == nil {
		return nil, fmt.Errorf("credential cache must not be nil")
	}
	return &Engine{cache: cache}, nil
}

// Execute runs a tool through the full credential + middleware pipeline.
//
// The tool's definition is loaded from the embedded definition set; an unknown
// tool (no embedded definition) returns an error. Credentials come from the
// supplied resolver, which must be non-nil; account selects which connected
// account's credential to resolve ("" = the resolver's default, design 003).
// Resolver-supplied credentials are always treated as ephemeral/managed (file
// injection writes a temp file and redirects via config_env / config_flag,
// then cleans up).
func (e *Engine) Execute(ctx context.Context, tool string, args []string, resolver credential.CredentialResolver, account string) (int, error) {
	if resolver == nil {
		return 1, fmt.Errorf("credential resolver must not be nil")
	}

	// 1. Load tool definition from the embedded set.
	def, err := loadDefinition(tool)
	if err != nil {
		return 1, err
	}

	mctx := &middleware.Context{
		Args: args,
		Env:  make(map[string]string),
	}

	// 1b. A help request never resolves credentials (design 335 D3):
	// capability discovery must work before a tool is connected, and help
	// makes no network call and touches no user data.
	//
	// For a service tool the question "is this a help request?" is answered
	// by cobra itself, off the in-process command tree, before any
	// credential work. A service rejects an empty credential env before
	// cobra ever sees the args, so help has to be answered here or not at
	// all.
	var svc tools.Service
	if def.Type == "service" {
		svc, err = tools.GetService(tool)
		if err != nil {
			return 1, err
		}
		handled, err := renderServiceHelp(svc, args)
		if err != nil {
			return 1, fmt.Errorf("render help for %q: %w", tool, err)
		}
		if handled {
			return 0, nil
		}
	}

	// Track whether this tool's credential bindings were actually resolved
	// and injected, for stale marking.
	hasCredentials := def.Auth != nil && len(def.Auth.Credentials) > 0
	if def.Type != "service" && isBareHelp(args) {
		// A binary passthrough has no in-process command tree — its help is
		// printed by the wrapped binary (gh, lark-cli), so cobra cannot be
		// asked. The strict fallback below is what keeps `<tool> --help`
		// reachable for an unconnected passthrough tool; anything less
		// literal than a bare help flag takes the normal path, because
		// without a tree there is nothing that could tell a help request
		// from a real invocation carrying the token "--help".
		hasCredentials = false
	}

	// 2-3. Resolve credentials and apply bindings.
	if hasCredentials {
		values, err := credential.ResolveBindings(ctx, e.cache, tool, account, def.Auth.Credentials, resolver)
		if err != nil {
			return 1, fmt.Errorf("credential resolution failed for %q: %w", tool, err)
		}

		injResult, err := credential.ApplyBindings(tool, def.Auth.Credentials, values)
		if err != nil {
			return 1, fmt.Errorf("credential injection failed for %q: %w", tool, err)
		}

		// Defer cleanup for ephemeral temp files (file inject).
		if injResult.Cleanup != nil {
			defer injResult.Cleanup()
		}

		// Merge injected env vars into mctx.Env.
		for k, v := range injResult.Env {
			mctx.Env[k] = v
		}

		// Append injected args (after user args, for subcommand-scoped flags).
		if len(injResult.Args) > 0 {
			mctx.Args = append(mctx.Args, injResult.Args...)
		}
	}

	// 4. If service type, delegate to built-in service. Help already
	// short-circuited above, so anything reaching here is a real invocation.
	if def.Type == "service" {
		result, err := svc.Execute(ctx, mctx.Args, mctx.Env)
		if result.CredentialRejected && hasCredentials {
			e.markCredentialsStale(tool, account)
		}
		return result.ExitCode, err
	}

	// 5. Resolve the real binary path.
	//
	// Note on design 335 D3's "help doesn't touch the network": that holds
	// exactly for service tools, which answer help off the in-process command
	// tree above. A binary passthrough's help is printed by the wrapped binary
	// itself, so `<tool> --help` on a cold runtime resolves — and may lazily
	// DOWNLOAD — the pinned release first. Skipping credential resolution is
	// what makes that help reachable at all before the tool is connected; the
	// download is the price of asking the real binary.
	binaryPath, err := ResolveBinary(ctx, def)
	if err != nil {
		return 1, fmt.Errorf("cannot find %q binary: %w", tool, err)
	}

	// 6. Run before hooks.
	if err := middleware.RunBefore(def.Before, mctx); err != nil {
		return 1, err
	}

	// 7-8. Execute binary.
	// If no after hooks, passthrough stdin/stdout/stderr directly (streaming).
	if len(def.After) == 0 {
		exitCode, err := executePassthrough(binaryPath, mctx.Args, mctx.Env)
		// 9. On non-zero exit, mark credentials stale.
		if exitCode != 0 && hasCredentials {
			e.markCredentialsStale(tool, account)
		}
		return exitCode, err
	}

	// With after hooks, capture output for processing.
	mctx.ExitCode, mctx.Stdout, mctx.Stderr, err = executeBuffered(binaryPath, mctx.Args, mctx.Env)
	rawExitCode := mctx.ExitCode // save before after-hooks can remap
	if err != nil && mctx.ExitCode == 0 {
		return 1, err
	}

	// 8. Run after hooks.
	if err := middleware.RunAfter(def.After, mctx); err != nil {
		return mctx.ExitCode, err
	}

	os.Stdout.Write(mctx.Stdout)
	os.Stderr.Write(mctx.Stderr)

	// 9. On non-zero raw exit (before after-hook remapping), mark credentials stale.
	if rawExitCode != 0 && hasCredentials {
		e.markCredentialsStale(tool, account)
	}

	return mctx.ExitCode, nil
}

// renderServiceHelp answers a service tool's help off its in-process cobra
// tree and reports whether it did. It returns false — meaning "not a help
// request, carry on" — unless cobra itself consumed a built-in -h/--help
// during the dry-run parse (design 335 D3). RunE is never invoked, so no
// provider call can happen on this path.
//
// The help that gets printed is the RESOLVED node's, not always the root's:
//
//   - root -> the flattened capability face, which lists every callable leaf
//     under the derived "(N — complete list)" claim (design 335 D2). cobra's
//     own root help lists direct children only, which is the false-absence
//     signal the whole design exists to remove.
//   - any deeper node -> cobra's own help for that node, which is what prints
//     the command's Long. Under design 335 D1 the Long is where the provider
//     API knowledge lives, and `<leaf> --help` is the only way to read it — so
//     an unconnected tool that could list its commands but never read their
//     prose would still be half-mute.
func renderServiceHelp(svc tools.Service, args []string) (bool, error) {
	root := svc.NewCommandTree()
	res, err := dryrun.Resolve(root, args)
	if err != nil {
		return false, err
	}
	if !res.Help {
		return false, nil
	}

	// Set the sink on the root: cobra's out/err writers are inherited down
	// the tree, so this covers whichever node ends up printing. Resolved here,
	// per call, so a swapped os.Stdout is honored.
	out := helpWriter()
	root.SetOut(out)
	root.SetErr(out)

	if note := pathTypoNote(root, res); note != "" {
		fmt.Fprintln(out, note)
	}
	if res.Cmd == root {
		return true, toolhelp.Render(out, root)
	}
	if err := res.Cmd.Help(); err != nil {
		return true, fmt.Errorf("cobra help for %q: %w", res.Cmd.CommandPath(), err)
	}
	return true, nil
}

// pathTypoNote returns the one-line signal that argv named a subcommand that
// does not exist ("" when it did not).
//
// `x post bogus --help` resolves to `post` with "bogus" left over, and cobra
// prints post's help with no indication that anything was dropped. By design
// 331's own premise a missing signal reads to an AI as a claim about the
// world — here, that `post bogus` exists and is what it is reading about. One
// line closes that.
//
// The note fires only when the resolved node HAS subcommands. On a leaf,
// leftover positionals are the command's own arguments (`messages get <id>
// --help`), not a mistyped path, and calling them "not a command" would be
// the false signal instead.
//
// A leftover positional is NOT sufficient on its own: the token must be
// verified absent from the WHOLE tree first. cobra's stripFlags runs inside
// Find BEFORE InitDefaultHelpFlag, so a bare `--help` has no NoOptDefVal yet
// and is taken for a flag that wants a value — it swallows the NEXT token
// during the subcommand search, and that token then survives the later
// ParseFlags as a leftover positional. `post --help search` therefore resolves
// to `post` with "search" left over even though `post search` exists.
//
// Checking only the RESOLVED node's children is not enough, because the theft
// can also change WHICH node argv resolves to: `--help contact list` loses
// "contact", the surviving "list" matches an unrelated top-level `list` group,
// and "contact" resurfaces as a positional under a node it was never meant for.
// It genuinely is not a child of `list` — and denying it would still be false,
// because `contact list` exists. So the suppression rule is existence ANYWHERE
// in the tree: a token that names a real command was almost certainly stolen by
// stripFlags, not mistyped. That trades a note on the rare genuine typo whose
// token happens to name a real command elsewhere (`post list --help` in a tree
// with a top-level `list`) for never printing a false-absence claim — the
// asymmetry design 335 asks for, since a false denial is read as a statement
// about the provider's coverage while silence is merely silence.
func pathTypoNote(root *cobra.Command, res dryrun.Resolution) string {
	if len(res.Args) == 0 || !res.Cmd.HasSubCommands() {
		return ""
	}
	name := res.Args[0]
	if name == "" || existsAnywhere(root, name) {
		return ""
	}
	// cobra injects `help` and `completion` into the root inside Execute,
	// AFTER the tree this dry-run walks was built — so they are absent here
	// but present at invocation time. `x help post` and `x completion bash`
	// really work; denying them would be the same false-absence claim.
	//
	// Deliberately not fixed by calling InitDefaultHelpCmd in dryrun.Resolve:
	// that would make `x help post` resolve to the injected help command,
	// flipping Inspect's Runnable to true with the default SideEffect=true,
	// and design 318's gate in helio-cli would start intercepting a help
	// invocation.
	if res.Cmd == root && isCobraInjectedName(name) {
		return ""
	}
	where := nodePath(root, res.Cmd)
	return fmt.Sprintf("note: %q is not a command under %s; showing help for %s.", name, where, where)
}

// existsAnywhere reports whether any command in the tree rooted at root answers
// to name, by its own name or by an alias. Hidden commands count: a hidden
// command still exists, so denying it would be just as false.
func existsAnywhere(root *cobra.Command, name string) bool {
	for _, sub := range root.Commands() {
		if sub.Name() == name || sub.HasAlias(name) {
			return true
		}
		if existsAnywhere(sub, name) {
			return true
		}
	}
	return false
}

// isCobraInjectedName reports whether name is one of the two commands cobra
// adds to a root by itself during Execute.
func isCobraInjectedName(name string) bool {
	return name == "help" || name == "completion"
}

// nodePath renders a resolved node the way a caller types it: the command path
// below the tool root, or the tool name itself at the root.
//
// Deliberately not toolhelp.LeafPath: that one renders the ROOT as its own
// call form ("[--browser <name>] -- <args>"), which is right for a capability
// row and wrong inside a sentence about where a subcommand was looked for.
func nodePath(root, cmd *cobra.Command) string {
	if cmd == root {
		return root.Name()
	}
	return strings.TrimPrefix(cmd.CommandPath(), root.CommandPath()+" ")
}

// isBareHelp reports whether args is nothing but literal built-in help flags.
//
// This is the binary-passthrough fallback ONLY: those tools exec a wrapped
// binary and expose no cobra tree, so the dry-run parse that answers the
// question properly for service tools is unavailable. The predicate is
// deliberately strict — a token-scanning "contains --help" would fire on
// `create --text "--help"`, on `create -- --help`, and on `--help=false`, and
// skipping credential resolution for those would turn a real, intended
// invocation into a silently unauthenticated one.
func isBareHelp(args []string) bool {
	if len(args) == 0 {
		return false
	}
	for _, arg := range args {
		if arg != "--help" && arg != "-h" {
			return false
		}
	}
	return true
}

// markCredentialsStale marks the cached credential for one (tool, account) as
// stale and prints a hint to stderr so the agent retries (triggering a
// re-resolve). Only the failing account's entry is touched.
func (e *Engine) markCredentialsStale(tool, account string) {
	e.cache.MarkStale(credential.CacheKey(tool, account))
	fmt.Fprintf(os.Stderr, "[anycli] credentials for %q may be stale. retry the same command to fetch fresh credentials.\n", tool)
}

// ResolveBinary finds the real binary path. An explicit absolute Resolve wins;
// otherwise the shared three-level resolution runs: pinned-versions dir, PATH
// (skipping the anycli shim directory), then lazy install for definitions with
// an official direct-download source. Definitions without one (e.g. lark-cli)
// keep the historical PATH-only behavior and error.
func ResolveBinary(ctx context.Context, def *registry.Definition) (string, error) {
	if def.Resolve != "" && def.Resolve != "which" {
		// Absolute path provided
		if _, err := os.Stat(def.Resolve); err != nil {
			return "", err
		}
		return def.Resolve, nil
	}
	return binresolve.Resolve(ctx, def.Name, def.Binary, def.Source, binresolve.Options{
		SkipPATHDir: config.BinDir(),
	})
}

// executePassthrough runs the binary with stdin/stdout/stderr connected directly.
func executePassthrough(binary string, args []string, env map[string]string) (int, error) {
	cmd := exec.Command(binary, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = buildEnv(env)

	err := cmd.Run()
	return cmd.ProcessState.ExitCode(), err
}

// executeBuffered runs the binary and captures output for after hooks.
func executeBuffered(binary string, args []string, env map[string]string) (int, []byte, []byte, error) {
	cmd := exec.Command(binary, args...)
	cmd.Stdin = os.Stdin

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = buildEnv(env)

	err := cmd.Run()
	return cmd.ProcessState.ExitCode(), stdout.Bytes(), stderr.Bytes(), err
}

func buildEnv(env map[string]string) []string {
	result := os.Environ()
	for k, v := range env {
		result = append(result, k+"="+v)
	}
	return result
}
