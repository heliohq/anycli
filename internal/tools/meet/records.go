package meet

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// addListFlags wires the shared list pagination flags with a per-resource
// default page size.
func addListFlags(cmd *cobra.Command, max *int, pageToken *string, def int) {
	cmd.Flags().IntVar(max, "max", def, "max results to return (page size)")
	cmd.Flags().StringVar(pageToken, "page-token", "", "page token from a previous list call")
}

// participant is the decoded shape of a conferenceRecords.participants entry.
type participant struct {
	Name              string `json:"name"`
	EarliestStartTime string `json:"earliestStartTime"`
	LatestEndTime     string `json:"latestEndTime"`
	SignedinUser      *struct {
		User        string `json:"user"`
		DisplayName string `json:"displayName"`
	} `json:"signedinUser"`
	AnonymousUser *struct {
		DisplayName string `json:"displayName"`
	} `json:"anonymousUser"`
	PhoneUser *struct {
		DisplayName string `json:"displayName"`
	} `json:"phoneUser"`
}

// displayName resolves the best human label for a participant, falling back to
// the resource name for participants the API gives no name for.
func (p participant) displayName() string {
	switch {
	case p.SignedinUser != nil && p.SignedinUser.DisplayName != "":
		return p.SignedinUser.DisplayName
	case p.AnonymousUser != nil && p.AnonymousUser.DisplayName != "":
		return p.AnonymousUser.DisplayName
	case p.PhoneUser != nil && p.PhoneUser.DisplayName != "":
		return p.PhoneUser.DisplayName
	default:
		return p.Name
	}
}

// buildRecordsFilter merges the convenience flags and the raw --filter into a
// single conferenceRecords.list EBNF expression (fields joined by AND). The
// convenience flags are not a second syntax — each expands to one native
// clause over the same filterable fields (space.name / space.meeting_code /
// start_time / end_time).
func buildRecordsFilter(space, after, before string, ongoing bool, raw string) string {
	var clauses []string
	if space != "" {
		if strings.HasPrefix(space, "spaces/") {
			clauses = append(clauses, fmt.Sprintf(`space.name = "%s"`, space))
		} else {
			clauses = append(clauses, fmt.Sprintf(`space.meeting_code = "%s"`, space))
		}
	}
	if after != "" {
		clauses = append(clauses, fmt.Sprintf(`start_time>="%s"`, after))
	}
	if before != "" {
		clauses = append(clauses, fmt.Sprintf(`start_time<="%s"`, before))
	}
	if ongoing {
		clauses = append(clauses, "end_time IS NULL")
	}
	if raw != "" {
		clauses = append(clauses, raw)
	}
	return strings.Join(clauses, " AND ")
}

func (s *Service) newRecordsListCmd(token string) *cobra.Command {
	var space, after, before, rawFilter, pageToken string
	var ongoing bool
	var max int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List conference records (post-meeting), newest first",
		Long: "List conference records (conferenceRecords.list), ordered by start time descending.\n" +
			"Convenience flags (--space/--after/--before/--ongoing) and --filter both expand into\n" +
			"the native EBNF filter over space.name / space.meeting_code / start_time / end_time.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if filter := buildRecordsFilter(space, after, before, ongoing, rawFilter); filter != "" {
				q.Set("filter", filter)
			}
			q.Set("pageSize", strconv.Itoa(max))
			if pageToken != "" {
				q.Set("pageToken", pageToken)
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/conferenceRecords", q, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				ConferenceRecords []struct {
					Name      string `json:"name"`
					StartTime string `json:"startTime"`
					EndTime   string `json:"endTime"`
					Space     string `json:"space"`
				} `json:"conferenceRecords"`
				NextPageToken string `json:"nextPageToken"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("meet: decode records list: %w", err)
			}
			if len(resp.ConferenceRecords) == 0 {
				fmt.Fprintln(s.stdout(), "no conference records")
				return nil
			}
			for _, r := range resp.ConferenceRecords {
				end := r.EndTime
				if end == "" {
					end = "ongoing"
				}
				fmt.Fprintf(s.stdout(), "%s\tstart=%s\tend=%s\tspace=%s\n", r.Name, r.StartTime, end, r.Space)
			}
			if resp.NextPageToken != "" {
				fmt.Fprintf(s.stdout(), "next page token: %s\n", resp.NextPageToken)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&space, "space", "", "restrict to a space (spaces/ID or a meeting code)")
	cmd.Flags().StringVar(&after, "after", "", "records started at/after this RFC3339 time")
	cmd.Flags().StringVar(&before, "before", "", "records started at/before this RFC3339 time")
	cmd.Flags().BoolVar(&ongoing, "ongoing", false, "only conferences still in progress (end_time IS NULL)")
	cmd.Flags().StringVar(&rawFilter, "filter", "", "raw conferenceRecords.list EBNF filter (ANDed with the convenience flags)")
	addListFlags(cmd, &max, &pageToken, 25)
	return cmd
}

func (s *Service) newRecordsGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <record>",
		Short: "Show one conference record: start/end/expire times and its space",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/"+recordName(args[0]), nil, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var r struct {
				Name       string `json:"name"`
				StartTime  string `json:"startTime"`
				EndTime    string `json:"endTime"`
				ExpireTime string `json:"expireTime"`
				Space      string `json:"space"`
			}
			if err := json.Unmarshal(body, &r); err != nil {
				return fmt.Errorf("meet: decode record: %w", err)
			}
			end := r.EndTime
			if end == "" {
				end = "ongoing"
			}
			fmt.Fprintf(s.stdout(),
				"Name:    %s\nStart:   %s\nEnd:     %s\nExpires: %s\nSpace:   %s\n",
				r.Name, r.StartTime, end, r.ExpireTime, r.Space)
			return nil
		},
	}
}

func (s *Service) newParticipantsListCmd(token string) *cobra.Command {
	var pageToken string
	var max int
	cmd := &cobra.Command{
		Use:   "list <record>",
		Short: "List participants of a conference record (who attended, and their overall window)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("pageSize", strconv.Itoa(max))
			if pageToken != "" {
				q.Set("pageToken", pageToken)
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/"+recordName(args[0])+"/participants", q, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				Participants  []participant `json:"participants"`
				NextPageToken string        `json:"nextPageToken"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("meet: decode participants list: %w", err)
			}
			if len(resp.Participants) == 0 {
				fmt.Fprintln(s.stdout(), "no participants")
				return nil
			}
			for _, p := range resp.Participants {
				end := p.LatestEndTime
				if end == "" {
					end = "still in call"
				}
				fmt.Fprintf(s.stdout(), "%s\t%s\tjoined=%s\tleft=%s\n", p.Name, p.displayName(), p.EarliestStartTime, end)
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

func (s *Service) newParticipantsSessionsCmd(token string) *cobra.Command {
	var pageToken string
	var max int
	cmd := &cobra.Command{
		Use:   "sessions <participant>",
		Short: "List a participant's join/leave sessions (participantSessions.list; segments across reconnects)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("pageSize", strconv.Itoa(max))
			if pageToken != "" {
				q.Set("pageToken", pageToken)
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/"+strings.TrimPrefix(args[0], "/")+"/participantSessions", q, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				ParticipantSessions []struct {
					Name      string `json:"name"`
					StartTime string `json:"startTime"`
					EndTime   string `json:"endTime"`
				} `json:"participantSessions"`
				NextPageToken string `json:"nextPageToken"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("meet: decode participant sessions: %w", err)
			}
			if len(resp.ParticipantSessions) == 0 {
				fmt.Fprintln(s.stdout(), "no sessions")
				return nil
			}
			for _, ps := range resp.ParticipantSessions {
				end := ps.EndTime
				if end == "" {
					end = "still in call"
				}
				fmt.Fprintf(s.stdout(), "%s\tstart=%s\tend=%s\n", ps.Name, ps.StartTime, end)
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
