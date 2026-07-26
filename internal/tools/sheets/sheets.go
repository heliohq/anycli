// Package sheets is the built-in Google Sheets service: a non-interactive
// cobra tree projecting the Sheets API v4 resource namespaces (spreadsheets /
// spreadsheets.values / spreadsheets.sheets) plus a small set of safe
// synthetic verbs (design 303). Ranges pass Sheets' native A1 notation through
// verbatim; the tabs.* verbs assemble the batchUpdate requests they map to, and
// `spreadsheets batch-update` is the raw escape hatch for everything else. A
// 401/403 very often means the token lacks a scope the user never granted —
// those errors carry an explicit reconnect hint.
//
// The tool works by ID: the Sheets API has no list/search (that is Drive), so
// every `<id>` position accepts either a bare spreadsheetId or a full
// docs.google.com URL, which the tool parses down to the id.
package sheets

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

// DefaultBaseURL is the production Sheets API v4 base.
const DefaultBaseURL = "https://sheets.googleapis.com/v4"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/sheets.json).
const EnvAccessToken = "SHEETS_ACCESS_TOKEN"

// scopeHint is appended to 401/403 errors: the usual cause is a token that
// lacks the spreadsheets scope the user never granted on connect.
const scopeHint = " (possibly missing scope — reconnect and grant access)"

// metaFields limits spreadsheets.get to title + per-tab properties (no grid
// data): the metadata every command needs to resolve tab names and report
// scale, without dumping cell contents.
const metaFields = "spreadsheetId,spreadsheetUrl,properties.title,sheets.properties(sheetId,title,index,gridProperties)"

// Service implements the built-in Sheets tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Sheets API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
	// sleep overrides the retry backoff sleeper; nil = time.Sleep. Tests
	// inject a recorder to keep retries deterministic and fast.
	sleep func(time.Duration)
}

// Execute runs one sheets subcommand with the resolved credentials in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		fmt.Fprintln(s.stderr(), "SHEETS_ACCESS_TOKEN is not set")
		return execution.Result{ExitCode: 1}, nil
	}
	root := s.newRoot(token)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(s.stderr(), err)
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
		Use:   "sheets",
		Short: "Google Sheets built-in service",
		Long: "Calls Sheets API v4 as the connected Google account, and works strictly by\n" +
			"id: the Sheets API has no list and no search — that is Drive — so a\n" +
			"spreadsheet is reachable only from a link the user supplied or from one\n" +
			"`spreadsheets create` just returned. Every <id> position accepts a bare\n" +
			"spreadsheetId or a full docs.google.com URL.\n" +
			"\n" +
			"Run `spreadsheets get` first on any unfamiliar file. It returns the tab\n" +
			"titles, their numeric gids and grid sizes with no cell data, and every\n" +
			"range in this tool names a tab. Guessing a tab title is the most common\n" +
			"way to earn a 400. Resolving --tab by title costs an extra metadata read\n" +
			"on each tabs command; passing the gid skips it.\n" +
			"\n" +
			"Ranges are Sheets' own A1 notation, passed through verbatim: `Sheet1!A1:D20`,\n" +
			"`Sheet1!B:B` for a whole column, `Sheet1!A1:D` for columns A-D from row 1\n" +
			"down. A tab title containing a space or non-ASCII text must be\n" +
			"single-quoted inside the range, as in `'Q3 Budget'!A1:D10`.\n" +
			"\n" +
			"Writes default to USER_ENTERED: every value is parsed as though a person\n" +
			"typed it into the UI, so `=SUM(A1:A3)` becomes a formula, `1/2` becomes a\n" +
			"date, and a leading + or 0 can disappear. Pass --raw when writing literal\n" +
			"text — phone numbers, ids, part numbers, anything starting with =, + or -.\n" +
			"\n" +
			"There is no undo. Drive version history can restore data but not\n" +
			"structure: a deleted tab turns every cross-tab formula that referenced it\n" +
			"into #REF! permanently.\n" +
			"\n" +
			"Output is a human-readable summary; --json emits the provider JSON\n" +
			"instead. Reads retry themselves on a 429 or 5xx (up to twice, with\n" +
			"backoff); writes never do, so a failed write surfaces rather than being\n" +
			"silently repeated. A 403 or 404 on a spreadsheet is almost always a\n" +
			"sharing problem — the file must be shared with the connected account —\n" +
			"rather than a missing scope.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON instead of the human-readable summary")

	spreadsheets := newGroupCmd("spreadsheets", "Spreadsheet metadata, creation, and the raw batchUpdate escape hatch")
	spreadsheets.AddCommand(
		s.newSpreadsheetsGetCmd(token),
		s.newSpreadsheetsCreateCmd(token),
		s.newSpreadsheetsBatchUpdateCmd(token),
	)

	values := newGroupCmd("values", "Read and write cell values by A1 range")
	values.AddCommand(
		s.newValuesGetCmd(token),
		s.newValuesUpdateCmd(token),
		s.newValuesAppendCmd(token),
		s.newValuesClearCmd(token),
	)

	tabs := newGroupCmd("tabs", "Manage tabs (sheets) within a spreadsheet")
	tabs.AddCommand(
		s.newTabsAddCmd(token),
		s.newTabsRenameCmd(token),
		s.newTabsDuplicateCmd(token),
		s.newTabsCopyToCmd(token),
		s.newTabsDeleteCmd(token),
	)

	root.AddCommand(spreadsheets, values, tabs)
	return root
}

// newGroupCmd is a runnable command group. cobra skips Args validation on
// non-runnable commands (help + exit 0 even for an unknown subcommand — a
// false success for an agent); making the group runnable restores it: bare
// group shows help, unknown subcommand fails.
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
