// Package meet is the built-in Google Meet service: a non-interactive cobra
// tree projecting the Meet REST API v2 resource namespaces (spaces /
// conferenceRecords and its participants / participantSessions / recordings /
// transcripts / transcripts.entries) plus the v2beta smartNotes read and the
// synthetic `transcripts text` verb that stitches paginated entries into a
// readable transcript (design 303 §Google Meet). Meet is read-heavy: the only
// high-blast-radius verb is `spaces end-conference`, which is gated by the
// skill's soft guardrail, not an approval gate (086 decided against). A
// 401/403 usually means the token lacks a scope the user never granted — those
// errors carry an explicit reconnect hint.
package meet

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

// DefaultBaseURL is the production Meet REST API v2 base.
const DefaultBaseURL = "https://meet.googleapis.com/v2"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/meet.json).
const EnvAccessToken = "MEET_ACCESS_TOKEN"

// scopeHint is appended to 401/403 errors: the usual cause is a token that
// lacks a scope the user never granted on connect.
const scopeHint = " (possibly missing scope — reconnect and grant access)"

// Service implements the built-in Meet tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Meet API v2 base; empty = DefaultBaseURL. Tests
	// point it at an httptest server. The v2beta base (smartNotes) is derived
	// from it via betaBase.
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

// Execute runs one meet subcommand with the resolved credentials in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		fmt.Fprintln(s.stderr(), "MEET_ACCESS_TOKEN is not set")
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

// betaBase is the v2beta base used only for the smartNotes resource: its
// get/list methods are GA, but Google still serves them under the /v2beta/
// URL. Derived from the v2 base so a test server override still routes
// correctly.
func (s *Service) betaBase() string {
	b := s.base()
	if strings.HasSuffix(b, "/v2") {
		return strings.TrimSuffix(b, "/v2") + "/v2beta"
	}
	return b + "beta"
}

func (s *Service) client() *http.Client {
	if s.HC != nil {
		return s.HC
	}
	return http.DefaultClient
}

func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "meet",
		Short:         "Google Meet built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON instead of the human-readable summary")

	records := newGroupCmd("records", "Conference records (post-meeting assets; API resource conferenceRecords)")
	records.AddCommand(s.newRecordsListCmd(token), s.newRecordsGetCmd(token))

	participants := newGroupCmd("participants", "Meeting participants (who attended, for how long)")
	participants.AddCommand(s.newParticipantsListCmd(token), s.newParticipantsSessionsCmd(token))

	recordings := newGroupCmd("recordings", "Recording artifacts (index + Drive exportUri; v1 does not download files)")
	recordings.AddCommand(s.newRecordingsListCmd(token))

	transcripts := newGroupCmd("transcripts", "Transcript artifacts and structured entries")
	transcripts.AddCommand(
		s.newTranscriptsListCmd(token),
		s.newTranscriptsEntriesCmd(token),
		s.newTranscriptsTextCmd(token),
	)

	smartNotes := newGroupCmd("smart-notes", "Smart notes artifacts (v2beta; index + Docs exportUri)")
	smartNotes.AddCommand(s.newSmartNotesListCmd(token))

	spaces := newGroupCmd("spaces", "Meeting spaces: create ad-hoc links, read/update config, end an active conference")
	spaces.AddCommand(
		s.newSpacesGetCmd(token),
		s.newSpacesCreateCmd(token),
		s.newSpacesUpdateCmd(token),
		s.newSpacesEndConferenceCmd(token),
	)

	root.AddCommand(records, participants, recordings, transcripts, smartNotes, spaces)
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

// recordName normalizes a conferenceRecord argument: bare id or full
// "conferenceRecords/{id}" both resolve to the canonical resource name.
func recordName(arg string) string {
	if strings.HasPrefix(arg, "conferenceRecords/") {
		return arg
	}
	return "conferenceRecords/" + arg
}

// spaceName normalizes a space argument. A bare id or a meeting code both map
// to "spaces/{arg}" — the API accepts the meeting code as a get alias in the
// {space} path segment.
func spaceName(arg string) string {
	if strings.HasPrefix(arg, "spaces/") {
		return arg
	}
	return "spaces/" + arg
}

// NewCommandTree returns the full command tree built with an empty token for
// dry-run parsing and traversal (tools.Service seam, design 318). The token
// is only captured by RunE closures, which are never run on this tree.
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot("") }
