package youtube

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// channelsGetPart is the default set of channel sections to hydrate.
const channelsGetPart = "snippet,statistics,contentDetails,status"

func (s *Service) newChannelsGetCmd(token string) *cobra.Command {
	var mine bool
	var id, forHandle, forUsername, part string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get channel info + statistics (subscriber / view / video counts)",
		Long: "Exactly one selector is required and they are tried in order: --mine, then\n" +
			"--id (comma-separated, several channels in one call), then --for-handle\n" +
			"(@Name), then --for-username for the legacy form. The statistics are\n" +
			"LIFETIME cumulative totals — subscribers, views, video count as of now —\n" +
			"not a time series, and windowed analytics live in a different API this\n" +
			"tool does not reach. A channel that hides its subscriber count returns an\n" +
			"empty value, printed as a dash. The contentDetails part also carries the\n" +
			"uploads playlist id that `videos mine` resolves internally.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("part", resolvePart(part, channelsGetPart))
			switch {
			case mine:
				q.Set("mine", "true")
			case id != "":
				q.Set("id", id)
			case forHandle != "":
				q.Set("forHandle", forHandle)
			case forUsername != "":
				q.Set("forUsername", forUsername)
			default:
				return &usageError{msg: "one of --mine, --id, --for-handle or --for-username is required"}
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/channels", q, nil)
			if err != nil {
				return err
			}
			lr, err := decodeList(body)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitList(lr)
			}
			return s.renderChannels(lr)
		},
	}
	cmd.Flags().BoolVar(&mine, "mine", false, "the authenticated user's own channel")
	cmd.Flags().StringVar(&id, "id", "", "comma-separated channel ids")
	cmd.Flags().StringVar(&forHandle, "for-handle", "", "channel handle, e.g. @HelioHQ")
	cmd.Flags().StringVar(&forUsername, "for-username", "", "legacy channel username")
	registerPartFlag(cmd, &part)
	return cmd
}

// renderChannels prints a compact human line per channel:
// title — subs / views / videos (id).
func (s *Service) renderChannels(lr listResponse) error {
	if len(lr.Items) == 0 {
		fmt.Fprintln(s.stdout(), "no channels")
		return nil
	}
	for _, raw := range lr.Items {
		var c struct {
			ID      string `json:"id"`
			Snippet struct {
				Title string `json:"title"`
			} `json:"snippet"`
			Statistics struct {
				SubscriberCount string `json:"subscriberCount"`
				ViewCount       string `json:"viewCount"`
				VideoCount      string `json:"videoCount"`
			} `json:"statistics"`
		}
		if err := json.Unmarshal(raw, &c); err != nil {
			return &apiError{msg: fmt.Sprintf("youtube: decode channel: %v", err), err: err}
		}
		fmt.Fprintf(s.stdout(), "%s — %s subs / %s views / %s videos (%s)\n",
			c.Snippet.Title, dash(c.Statistics.SubscriberCount), dash(c.Statistics.ViewCount),
			dash(c.Statistics.VideoCount), c.ID)
	}
	return nil
}

// dash renders a missing counter (hidden subscriber counts return "") as "-".
func dash(v string) string {
	if v == "" {
		return "-"
	}
	return v
}
