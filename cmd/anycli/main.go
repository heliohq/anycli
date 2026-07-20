// Command anycli is the standalone dev harness for tool-definition
// development (the embeddable library stays the product; this binary is a
// development aid). It executes an embedded tool definition against the real
// provider API with credentials taken from ANYCLI_CRED_* environment
// variables — no Helio services involved.
//
//	anycli list
//	ANYCLI_CRED_ACCESS_TOKEN=... anycli slack -- chat send --channel C1 --text hi
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	anycli "github.com/heliohq/anycli"
	"github.com/heliohq/anycli/definitions"
)

func main() {
	os.Exit(run(os.Args[1:], os.Environ()))
}

func run(args, environ []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		usage(os.Stderr)
		return 2
	}
	if args[0] == "list" {
		return runList(os.Stdout)
	}
	tool, toolArgs, err := parseInvocation(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "anycli:", err)
		usage(os.Stderr)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	engine, err := anycli.New(anycli.Config{})
	if err != nil {
		fmt.Fprintln(os.Stderr, "anycli:", err)
		return 1
	}
	exit, err := engine.ExecuteWith(ctx, anycli.Tool(tool), toolArgs, newEnvResolver(environ), anycli.ExecOptions{})
	if err != nil {
		fmt.Fprintln(os.Stderr, "anycli:", err)
		if exit == 0 {
			exit = 1
		}
	}
	return exit
}

// parseInvocation splits `<tool> -- <args…>`. Everything after `--` passes
// to the tool verbatim (same end-of-options convention as heliox tool); the
// separator is mandatory so tool flags can never collide with harness flags.
func parseInvocation(args []string) (tool string, toolArgs []string, err error) {
	if len(args) == 0 {
		return "", nil, fmt.Errorf("no tool named")
	}
	tool = args[0]
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		if rest[i] == "--" {
			return tool, rest[i+1:], nil
		}
		return "", nil, fmt.Errorf("unexpected argument %q before `--` (tool args go after `--`)", rest[i])
	}
	return "", nil, fmt.Errorf("missing `--` separator: anycli %s -- <args…>", tool)
}

func runList(w *os.File) int {
	defs, err := definitions.ListBundled()
	if err != nil {
		fmt.Fprintln(os.Stderr, "anycli:", err)
		return 1
	}
	for _, d := range defs {
		kind := d.Type
		if kind == "" {
			kind = "cli"
		}
		fmt.Fprintf(w, "%-22s %-8s %s\n", d.Name, kind, d.Description)
	}
	return 0
}

func usage(w *os.File) {
	fmt.Fprint(w, `anycli — dev harness for embedded tool definitions

Usage:
  anycli list
  anycli <tool> -- <args…>

Credentials come from ANYCLI_CRED_* environment variables; the suffix
(lowercased) is the credential field name, e.g.
  ANYCLI_CRED_ACCESS_TOKEN → access_token
  ANYCLI_CRED_ACCOUNT_KEY  → account_key
`)
}
