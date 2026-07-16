// Package slides is the built-in Google Slides service: a non-interactive
// cobra tree projecting the Slides API v1 resource surface (presentations get
// / create / batchUpdate + presentations.pages get / getThumbnail) plus
// synthetic verbs named after the batchUpdate request families (slides / text
// / images / elements). Every write ultimately funnels through batchUpdate;
// `batch-update` passes the full 44-request surface through verbatim (design
// 303). A 401/403 very often means the token lacks a scope the user never
// granted — those errors carry an explicit reconnect hint.
package slides

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// DefaultBaseURL is the production Slides API base.
const DefaultBaseURL = "https://slides.googleapis.com/v1"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/slides.json).
const EnvAccessToken = "SLIDES_ACCESS_TOKEN"

// scopeHint is appended to 401/403 errors: the usual cause is a token that
// lacks a scope the user never granted on connect (e.g. an escape-hatch
// batch-update calling a Sheets-linked-chart request the presentations scope
// does not cover).
const scopeHint = " (possibly missing scope — reconnect and grant access)"

// Service implements the built-in Google Slides tool. It satisfies
// tools.Service by duck typing (this package never imports the registry — no
// import cycle).
type Service struct {
	// BaseURL overrides the Slides API base; empty = DefaultBaseURL. Tests
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
	// genID overrides object-id generation; nil = random hex. Tests inject a
	// deterministic generator to assert exact batchUpdate bodies.
	genID func(prefix string) string
}

// Execute runs one slides subcommand with the resolved credentials in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		fmt.Fprintln(s.stderr(), "SLIDES_ACCESS_TOKEN is not set")
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

// newObjectID mints a Slides object id (5–50 chars, starts alphanumeric). The
// synthetic verbs assign known ids up front so a single batchUpdate can both
// create a placeholder and fill it without a round trip to learn its id.
func (s *Service) newObjectID(prefix string) string {
	if s.genID != nil {
		return s.genID(prefix)
	}
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failing is unrecoverable here; fall back to a
		// time-based id so the command still produces a valid object id.
		return fmt.Sprintf("%s%x", prefix, time.Now().UnixNano())
	}
	return prefix + hex.EncodeToString(b[:])
}

func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "slides",
		Short:         "Google Slides built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output raw API JSON instead of the human-readable summary")

	presentations := newGroupCmd("presentations", "Presentations (get outline / create)")
	presentations.AddCommand(s.newPresentationsGetCmd(token), s.newPresentationsCreateCmd(token))

	pages := newGroupCmd("pages", "Pages (single-page element tree / thumbnail)")
	pages.AddCommand(s.newPagesGetCmd(token), s.newPagesThumbnailCmd(token))

	slidesGrp := newGroupCmd("slides", "Slides (add / duplicate / move / delete a page)")
	slidesGrp.AddCommand(
		s.newSlidesAddCmd(token),
		s.newSlidesDuplicateCmd(token),
		s.newSlidesMoveCmd(token),
		s.newSlidesDeleteCmd(token),
	)

	text := newGroupCmd("text", "Text (insert / replace / delete)")
	text.AddCommand(s.newTextInsertCmd(token), s.newTextReplaceCmd(token), s.newTextDeleteCmd(token))

	images := newGroupCmd("images", "Images (insert from a public URL)")
	images.AddCommand(s.newImagesInsertCmd(token))

	elements := newGroupCmd("elements", "Elements (delete a page element)")
	elements.AddCommand(s.newElementsDeleteCmd(token))

	root.AddCommand(presentations, pages, slidesGrp, text, images, elements, s.newBatchUpdateCmd(token))
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

// extractPresentationID accepts either a bare presentation id or a full Slides
// URL (https://docs.google.com/presentation/d/<id>/edit) and returns the id.
// "User pastes the link" is the most natural input shape (design 303).
func extractPresentationID(raw string) string {
	raw = strings.TrimSpace(raw)
	if i := strings.Index(raw, "/d/"); i >= 0 {
		rest := raw[i+len("/d/"):]
		if j := strings.IndexAny(rest, "/?#"); j >= 0 {
			rest = rest[:j]
		}
		if rest != "" {
			return rest
		}
	}
	return raw
}

// presentationURL is the canonical editor link for a presentation id.
func presentationURL(id string) string {
	return "https://docs.google.com/presentation/d/" + id + "/edit"
}
