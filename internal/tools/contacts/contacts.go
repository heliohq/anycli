// Package contacts is the built-in Google Contacts service: a non-interactive
// cobra tree projecting the People API v1 resource namespaces
// (people.connections / people / otherContacts / contactGroups) plus the
// synthetic `resolve` verb that answers "what is X's address" in one call by
// merging My Contacts and Other Contacts (design 303 §Google Contacts). v1 is
// read-only. Search flags pass the People API's native prefix-phrase query
// semantics through verbatim. A 401/403 very often means the token lacks a
// scope the user never granted — those errors carry an explicit reconnect hint.
package contacts

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// DefaultBaseURL is the production People API base.
const DefaultBaseURL = "https://people.googleapis.com/v1"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/contacts.json).
const EnvAccessToken = "CONTACTS_ACCESS_TOKEN"

// scopeHint is appended to 401/403 errors: the usual cause is a token that
// lacks a scope the user never granted on connect.
const scopeHint = " (possibly missing scope — reconnect and grant access)"

// resolveMissExitCode is the exit code `resolve` returns when a name matched no
// contact. It is distinct from a hard failure (1) so the caller can tell an
// empty-but-successful lookup apart from an error.
const resolveMissExitCode = 2

// Default field masks. People API splits the mask parameter across endpoints:
// people.* use personFields, search/otherContacts use readMask. Other Contacts
// carry a naturally narrow field set (no organizations/nicknames).
const (
	defaultPersonFields = "names,emailAddresses,phoneNumbers,organizations"
	otherReadMask       = "names,emailAddresses,phoneNumbers"
)

// searchPageCap is the People API hard upper bound on searchContacts /
// otherContacts.search pageSize (values above 30 are silently capped).
const searchPageCap = 30

// warmupDelay is the pause between the empty warmup query and the real search
// query: People API search is a lazy cache the docs require priming first.
const warmupDelay = 1200 * time.Millisecond

// Service implements the built-in Google Contacts tool. It satisfies
// tools.Service by duck typing (this package never imports the registry — no
// import cycle).
type Service struct {
	// BaseURL overrides the People API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
	// sleep overrides the retry/warmup sleeper; nil = time.Sleep. Tests inject
	// a recorder to keep timing deterministic and fast.
	sleep func(time.Duration)
	// resolveMiss is set by `resolve` when a name matched nothing. Execute
	// reads it to surface resolveMissExitCode without treating it as an error.
	resolveMiss bool
}

// Execute runs one contacts subcommand with the resolved credentials in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		fmt.Fprintln(s.stderr(), "CONTACTS_ACCESS_TOKEN is not set")
		return execution.Result{ExitCode: 1}, nil
	}
	root := s.newRoot(token)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(s.stderr(), err)
		return execution.Failure(err), nil
	}
	if s.resolveMiss {
		return execution.Result{ExitCode: resolveMissExitCode}, nil
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

func (s *Service) base() string {
	if s.BaseURL != "" {
		return strings.TrimRight(s.BaseURL, "/")
	}
	return DefaultBaseURL
}

func (s *Service) client() *http.Client {
	if s.HC != nil {
		return s.HC
	}
	return http.DefaultClient
}

func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:   "contacts",
		Short: "Google Contacts built-in service (read-only)",
		Long: "Read-only. Nothing here creates, edits, labels or deletes a contact, and\n" +
			"no other write path substitutes — changing the address book is something\n" +
			"the person does in Google Contacts.\n" +
			"\n" +
			"Two separate stores sit behind this tool. My Contacts are people the\n" +
			"user deliberately saved (`list`, `search`); Other Contacts are addresses\n" +
			"Gmail auto-collected from correspondence (`other list`,\n" +
			"`other search`), and the API gives those a narrower field set — names,\n" +
			"emails and phones, never organizations. `resolve` queries both in one\n" +
			"call and tags every match with its source, which makes it the right\n" +
			"first move for \"what is X's address\".\n" +
			"\n" +
			"Search matches PREFIX PHRASES, not substrings: \"foo name\" is found by\n" +
			"f, foo, foo n or nam, but not by \"oo n\". After a miss, retry with a\n" +
			"SHORTER prefix such as the surname alone — never a fragment from the\n" +
			"middle of a word, and never the identical query twice.\n" +
			"\n" +
			"The People API's search index is a lazy cache, so each search command\n" +
			"sends a priming request and waits about a second before the real query;\n" +
			"searching is inherently slower than listing. Propagation is\n" +
			"minute-scale, so a contact saved moments ago can be genuinely\n" +
			"unfindable by search while `list --sort last-modified` already shows it.\n" +
			"\n" +
			"Output defaults to one line per contact; --json returns the API payload.\n" +
			"A 401 or 403 usually means the token was never granted the scope the\n" +
			"command needs.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON instead of the human-readable summary")

	other := newGroupCmd("other", "Other Contacts (addresses Gmail auto-collected)")
	other.AddCommand(s.newOtherListCmd(token), s.newOtherSearchCmd(token))

	groups := newGroupCmd("groups", "Contact groups (labels)")
	groups.AddCommand(s.newGroupsListCmd(token), s.newGroupsGetCmd(token))

	root.AddCommand(
		s.newListCmd(token),
		s.newGetCmd(token),
		s.newSearchCmd(token),
		other,
		groups,
		s.newResolveCmd(token),
	)
	return root
}

// newGroupCmd is a runnable command group. cobra skips Args validation on
// non-runnable commands (help + exit 0 even for an unknown subcommand — a false
// success for an agent); making the group runnable restores it: bare group
// shows help, unknown subcommand fails.
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}

// jsonOut reports whether the persistent --json flag was set for cmd.
func jsonOut(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("json")
	return v
}

// NewCommandTree returns the full command tree built with an empty token for
// dry-run parsing and traversal (tools.Service seam, design 318). The token
// is only captured by RunE closures, which are never run on this tree.
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot("") }
