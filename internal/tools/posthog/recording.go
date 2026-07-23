package posthog

import (
	"github.com/spf13/cobra"
)

// newRecordingCmd groups session-recording metadata read access (list only;
// recording playback bytes are out of scope for a CLI tool).
func (s *Service) newRecordingCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "recording", Short: "Session recordings (list metadata)"}
	cmd.AddCommand(
		s.newProjectListCmd(token, "list", "List session recordings (GET /api/projects/<id>/session_recordings/)", "/session_recordings/", false),
	)
	return cmd
}
