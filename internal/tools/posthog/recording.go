package posthog

import (
	"github.com/spf13/cobra"
)

// longRecordingList sits next to the group because the leaf is built by the
// shared project-list constructor.
const longRecordingList = "Returns recording METADATA — id, person, start time, duration — and never\n" +
	"the recorded events or a playback URL; there is no command that fetches\n" +
	"either. Filtering is `--limit` / `--offset` only, with no `--search`, so a\n" +
	"recording is located by paging rather than by query."

// newRecordingCmd groups session-recording metadata read access (list only;
// recording playback bytes are out of scope for a CLI tool).
func (s *Service) newRecordingCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "recording", Short: "Session recordings (list metadata)"}
	cmd.AddCommand(
		s.newProjectListCmd(token, "list", "List session recordings (GET /api/projects/<id>/session_recordings/)", longRecordingList, "/session_recordings/", false),
	)
	return cmd
}
