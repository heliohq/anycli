package meet

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// artifactPageSize is the page size requested for the single-page
// recordings/transcripts/smartNotes lists. A conference record carries at most
// a handful of each, but the provider default is only 10 (cap 100). Asking for
// the cap makes silent truncation effectively impossible; on the rare overflow
// the nextPageToken is surfaced as a hint rather than dropped. These three
// intentionally expose no --max/--page-token flags (design 303 §命令面).
const artifactPageSize = 100

// listAll paginates a GET list endpoint to exhaustion, invoking collect with
// each page body; collect returns the page's nextPageToken ("" ends it).
func (s *Service) listAll(ctx context.Context, token, path string, pageSize int, collect func([]byte) (string, error)) error {
	pageToken := ""
	for {
		q := url.Values{}
		q.Set("pageSize", strconv.Itoa(pageSize))
		if pageToken != "" {
			q.Set("pageToken", pageToken)
		}
		body, err := s.call(ctx, token, http.MethodGet, path, q, nil)
		if err != nil {
			return err
		}
		next, err := collect(body)
		if err != nil {
			return err
		}
		if next == "" {
			return nil
		}
		pageToken = next
	}
}

func (s *Service) newRecordingsListCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "list <record>",
		Short: "List recordings of a conference record (state + Drive fileId + exportUri; v1 does not download the file)",
		Args:  cobra.ExactArgs(1),
		// GET /conferenceRecords/{r}/recordings — read-only (design 318).
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Single-page GET (no pageToken loop): a conference record almost
			// always has 0–2 recordings. Request the 100 cap so the provider's
			// default of 10 can't silently truncate; a nextPageToken (which
			// should never appear) is surfaced as a hint below.
			q := url.Values{}
			q.Set("pageSize", strconv.Itoa(artifactPageSize))
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/"+recordName(args[0])+"/recordings", q, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				Recordings []struct {
					Name             string `json:"name"`
					State            string `json:"state"`
					DriveDestination struct {
						File      string `json:"file"`
						ExportURI string `json:"exportUri"`
					} `json:"driveDestination"`
				} `json:"recordings"`
				NextPageToken string `json:"nextPageToken"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("meet: decode recordings list: %w", err)
			}
			if len(resp.Recordings) == 0 {
				fmt.Fprintln(s.stdout(), "no recordings")
				return nil
			}
			for _, r := range resp.Recordings {
				fmt.Fprintf(s.stdout(), "%s\tstate=%s\tfile=%s\t%s\n", r.Name, r.State, r.DriveDestination.File, r.DriveDestination.ExportURI)
			}
			if resp.NextPageToken != "" {
				fmt.Fprintf(s.stdout(), "more than %d recordings; some were not shown\n", artifactPageSize)
			}
			return nil
		},
	}
}

func (s *Service) newTranscriptsListCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "list <record>",
		Short: "List transcripts of a conference record (state + Docs documentId + exportUri)",
		Args:  cobra.ExactArgs(1),
		// GET /conferenceRecords/{r}/transcripts — read-only (design 318).
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Single-page GET (see recordings list): transcripts per record are
			// a tiny fixed set. Request the 100 cap so the provider default of 10
			// can't silently truncate; a nextPageToken is surfaced as a hint.
			q := url.Values{}
			q.Set("pageSize", strconv.Itoa(artifactPageSize))
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/"+recordName(args[0])+"/transcripts", q, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				Transcripts []struct {
					Name            string `json:"name"`
					State           string `json:"state"`
					DocsDestination struct {
						Document  string `json:"document"`
						ExportURI string `json:"exportUri"`
					} `json:"docsDestination"`
				} `json:"transcripts"`
				NextPageToken string `json:"nextPageToken"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("meet: decode transcripts list: %w", err)
			}
			if len(resp.Transcripts) == 0 {
				fmt.Fprintln(s.stdout(), "no transcripts")
				return nil
			}
			for _, t := range resp.Transcripts {
				fmt.Fprintf(s.stdout(), "%s\tstate=%s\tdoc=%s\t%s\n", t.Name, t.State, t.DocsDestination.Document, t.DocsDestination.ExportURI)
			}
			if resp.NextPageToken != "" {
				fmt.Fprintf(s.stdout(), "more than %d transcripts; some were not shown\n", artifactPageSize)
			}
			return nil
		},
	}
}

func (s *Service) newTranscriptsEntriesCmd(token string) *cobra.Command {
	var pageToken string
	var max int
	cmd := &cobra.Command{
		Use:   "entries <transcript>",
		Short: "List structured transcript entries (speaker resource + text + timestamps), oldest first",
		Args:  cobra.ExactArgs(1),
		// GET .../transcripts/{t}/entries — read-only (design 318).
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("pageSize", strconv.Itoa(max))
			if pageToken != "" {
				q.Set("pageToken", pageToken)
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/"+strings.TrimPrefix(args[0], "/")+"/entries", q, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				TranscriptEntries []transcriptEntry `json:"transcriptEntries"`
				NextPageToken     string            `json:"nextPageToken"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("meet: decode transcript entries: %w", err)
			}
			if len(resp.TranscriptEntries) == 0 {
				fmt.Fprintln(s.stdout(), "no entries")
				return nil
			}
			for _, e := range resp.TranscriptEntries {
				fmt.Fprintf(s.stdout(), "%s\t%s\t%s\n", e.StartTime, e.Participant, e.Text)
			}
			if resp.NextPageToken != "" {
				fmt.Fprintf(s.stdout(), "next page token: %s\n", resp.NextPageToken)
			}
			return nil
		},
	}
	addListFlags(cmd, &max, &pageToken, 100)
	return cmd
}

// transcriptEntry is one row of conferenceRecords.transcripts.entries. The
// participant field is only a resource name — text synthesis joins it to a
// display name via the record's participants list.
type transcriptEntry struct {
	Name        string `json:"name"`
	Participant string `json:"participant"`
	Text        string `json:"text"`
	StartTime   string `json:"startTime"`
	EndTime     string `json:"endTime"`
}

// textLine is one synthesized speaker line in the transcript text output.
type textLine struct {
	Speaker   string `json:"speaker"`
	Text      string `json:"text"`
	StartTime string `json:"startTime"`
}

func (s *Service) newTranscriptsTextCmd(token string) *cobra.Command {
	var save string
	cmd := &cobra.Command{
		Use:   "text <transcript>",
		Short: "Stitch all transcript entries into readable `speaker: text` lines (paginates + resolves speaker names)",
		Long: "Synthetic verb (no direct API endpoint): pages through every transcript entry,\n" +
			"resolves each speaker's display name from the record's participants, orders by\n" +
			"start time, and prints `speaker: text` lines. Source is the Meet API entries, which\n" +
			"can differ slightly from the Google Docs transcript file.",
		Args: cobra.ExactArgs(1),
		// Synthetic read: GET participants + GET entries only; --save writes a
		// local file, never a provider mutation (design 318).
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			transcript := strings.TrimPrefix(args[0], "/")
			record, ok := recordOfTranscript(transcript)
			if !ok {
				return fmt.Errorf("meet: %q is not a transcript resource name (want conferenceRecords/{r}/transcripts/{t})", args[0])
			}
			names, err := s.participantNames(cmd.Context(), token, record)
			if err != nil {
				return err
			}
			entries, err := s.allTranscriptEntries(cmd.Context(), token, transcript)
			if err != nil {
				return err
			}
			sort.SliceStable(entries, func(i, j int) bool { return entries[i].StartTime < entries[j].StartTime })
			lines := make([]textLine, 0, len(entries))
			for _, e := range entries {
				speaker := names[e.Participant]
				if speaker == "" {
					speaker = e.Participant
				}
				lines = append(lines, textLine{Speaker: speaker, Text: e.Text, StartTime: e.StartTime})
			}
			if jsonOut(cmd) {
				return s.emitJSON(map[string]any{"transcript": transcript, "source": "meet-api-entries", "lines": lines})
			}
			var b strings.Builder
			for _, l := range lines {
				fmt.Fprintf(&b, "%s: %s\n", l.Speaker, l.Text)
			}
			if save != "" {
				if err := os.WriteFile(save, []byte(b.String()), 0o644); err != nil {
					return fmt.Errorf("meet: save transcript: %w", err)
				}
				fmt.Fprintf(s.stdout(), "saved %d line(s) to %s\n", len(lines), save)
				return nil
			}
			if len(lines) == 0 {
				fmt.Fprintln(s.stdout(), "no transcript entries")
				return nil
			}
			_, err = s.stdout().Write([]byte(b.String()))
			return err
		},
	}
	cmd.Flags().StringVar(&save, "save", "", "write the transcript text to this file instead of stdout")
	return cmd
}

// recordOfTranscript extracts the parent conferenceRecords/{r} name from a
// transcript resource name.
func recordOfTranscript(transcript string) (string, bool) {
	i := strings.Index(transcript, "/transcripts/")
	if i <= 0 || !strings.HasPrefix(transcript, "conferenceRecords/") {
		return "", false
	}
	return transcript[:i], true
}

// participantNames returns a map from participant resource name to display
// name for every participant of a conference record.
func (s *Service) participantNames(ctx context.Context, token, record string) (map[string]string, error) {
	names := map[string]string{}
	err := s.listAll(ctx, token, "/"+record+"/participants", 250, func(body []byte) (string, error) {
		var resp struct {
			Participants  []participant `json:"participants"`
			NextPageToken string        `json:"nextPageToken"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return "", fmt.Errorf("meet: decode participants: %w", err)
		}
		for _, p := range resp.Participants {
			names[p.Name] = p.displayName()
		}
		return resp.NextPageToken, nil
	})
	return names, err
}

// allTranscriptEntries pages through every entry of a transcript.
func (s *Service) allTranscriptEntries(ctx context.Context, token, transcript string) ([]transcriptEntry, error) {
	var entries []transcriptEntry
	err := s.listAll(ctx, token, "/"+transcript+"/entries", 100, func(body []byte) (string, error) {
		var resp struct {
			TranscriptEntries []transcriptEntry `json:"transcriptEntries"`
			NextPageToken     string            `json:"nextPageToken"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return "", fmt.Errorf("meet: decode transcript entries: %w", err)
		}
		entries = append(entries, resp.TranscriptEntries...)
		return resp.NextPageToken, nil
	})
	return entries, err
}

func (s *Service) newSmartNotesListCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "list <record>",
		Short: "List smart notes of a conference record (v2beta; state + Docs documentId + exportUri)",
		Args:  cobra.ExactArgs(1),
		// GET v2beta /conferenceRecords/{r}/smartNotes — read-only (design 318).
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// smartNotes get/list are GA methods, but still served under the
			// /v2beta/ URL — route via betaBase. Single-page GET: a record has at
			// most a handful of smart-notes files. Request the 100 cap so the
			// provider default of 10 can't silently truncate; a nextPageToken is
			// surfaced as a hint.
			q := url.Values{}
			q.Set("pageSize", strconv.Itoa(artifactPageSize))
			body, err := s.callBase(cmd.Context(), s.betaBase(), token, http.MethodGet, "/"+recordName(args[0])+"/smartNotes", q, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				SmartNotes []struct {
					Name            string `json:"name"`
					State           string `json:"state"`
					DocsDestination struct {
						Document  string `json:"document"`
						ExportURI string `json:"exportUri"`
					} `json:"docsDestination"`
				} `json:"smartNotes"`
				NextPageToken string `json:"nextPageToken"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("meet: decode smart notes list: %w", err)
			}
			if len(resp.SmartNotes) == 0 {
				fmt.Fprintln(s.stdout(), "no smart notes")
				return nil
			}
			for _, n := range resp.SmartNotes {
				fmt.Fprintf(s.stdout(), "%s\tstate=%s\tdoc=%s\t%s\n", n.Name, n.State, n.DocsDestination.Document, n.DocsDestination.ExportURI)
			}
			if resp.NextPageToken != "" {
				fmt.Fprintf(s.stdout(), "more than %d smart notes; some were not shown\n", artifactPageSize)
			}
			return nil
		},
	}
}
