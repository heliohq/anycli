// Package drive is the built-in Google Drive service: a non-interactive cobra
// tree projecting the Drive API v3 resource namespaces (files / permissions /
// about) plus a few safe synthetic verbs (mkdir / trash / untrash / share).
// Search flags pass Drive's native `q` query syntax through verbatim.
//
// v1 runs on the non-sensitive drive.file scope (design 303 §Google Drive): the
// tool only sees files it created or the user explicitly granted it — a 404 on
// an existing file is the scope boundary, not a bug. A 401/403 very often means
// the token lacks a scope the user never granted; those errors carry an
// explicit reconnect hint.
package drive

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

// DefaultBaseURL is the production Drive API base (metadata + JSON resources).
const DefaultBaseURL = "https://www.googleapis.com/drive/v3"

// DefaultUploadBaseURL is the production Drive media-upload base. Uploads use a
// separate host path (/upload/drive/v3) from every other Drive endpoint.
const DefaultUploadBaseURL = "https://www.googleapis.com/upload/drive/v3"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/drive.json).
const EnvAccessToken = "DRIVE_ACCESS_TOKEN"

// scopeHint is appended to 401/403 errors: the usual cause is a token that
// lacks a scope the user never granted on connect.
const scopeHint = " (possibly missing scope — reconnect and grant access)"

// visibilityHint is appended to 404 errors on files/get, download, export, and
// permissions: under drive.file the tool only sees files it created or the user
// explicitly granted it. A 404 on an existing file is that boundary, not a bug.
const visibilityHint = " (file is outside this tool's authorization — drive.file only sees files Helio created or the user explicitly shared with it; do not retry)"

// defaultResumableThreshold is the media size above which upload switches from
// a single multipart request to a resumable session (design 303 §upload).
const defaultResumableThreshold int64 = 5 * 1024 * 1024

// Service implements the built-in Google Drive tool. It satisfies tools.Service
// by duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Drive API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// UploadBaseURL overrides the media-upload base; empty = DefaultUploadBaseURL.
	UploadBaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
	// sleep overrides the retry backoff sleeper; nil = time.Sleep. Tests inject
	// a recorder to keep retries deterministic and fast.
	sleep func(time.Duration)
	// resumableThreshold overrides the multipart/resumable upload boundary in
	// bytes; 0 = defaultResumableThreshold. Tests set a tiny value to exercise
	// the resumable path without a large file.
	resumableThreshold int64
}

// Execute runs one drive subcommand with the resolved credentials in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		fmt.Fprintln(s.stderr(), "DRIVE_ACCESS_TOKEN is not set")
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

func (s *Service) uploadBase() string {
	if s.UploadBaseURL != "" {
		return strings.TrimRight(s.UploadBaseURL, "/")
	}
	return DefaultUploadBaseURL
}

func (s *Service) client() *http.Client {
	if s.HC != nil {
		return s.HC
	}
	return http.DefaultClient
}

func (s *Service) uploadCutover() int64 {
	if s.resumableThreshold > 0 {
		return s.resumableThreshold
	}
	return defaultResumableThreshold
}

func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "drive",
		Short:         "Google Drive built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON instead of the human-readable summary")

	files := newGroupCmd("files", "Files")
	files.AddCommand(
		s.newFilesListCmd(token),
		s.newFilesGetCmd(token),
		s.newFilesDownloadCmd(token),
		s.newFilesExportCmd(token),
		s.newFilesUploadCmd(token),
		s.newFilesMkdirCmd(token),
		s.newFilesUpdateCmd(token),
		s.newFilesCopyCmd(token),
		s.newFilesTrashCmd(token, false),
		s.newFilesTrashCmd(token, true),
		s.newFilesShareCmd(token),
	)

	permissions := newGroupCmd("permissions", "Permissions (sharing)")
	permissions.AddCommand(
		s.newPermissionsListCmd(token),
		s.newPermissionsUpdateCmd(token),
		s.newPermissionsDeleteCmd(token),
	)

	root.AddCommand(s.newAboutCmd(token), files, permissions)
	return root
}

// newGroupCmd is a runnable command group. cobra skips Args validation on
// non-runnable commands (help + exit 0 even for an unknown subcommand — a false
// success for an agent); making the group runnable restores it: a bare group
// shows help, an unknown subcommand fails.
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
