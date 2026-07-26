package semrush

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// unitsPath is the free API-units balance endpoint (0 units, no report data).
const unitsPath = "/users/countapiunits.html"

// newUnitsCmd checks the account's remaining API-unit balance. It is free (0
// units) and a good habit before a large report pull, since every report line
// debits the shared account balance.
func (s *Service) newUnitsCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:   "units",
		Short: "Remaining Semrush API-unit balance (free check)",
		Long: "Costs 0 units and touches no report endpoint, so it is always safe to run\n" +
			"first. Emits `{\"api_units_remaining\": <n>}` for the whole subscription —\n" +
			"the balance is shared across every key on the account and is spent by any\n" +
			"other Semrush integration too, so a number that dropped since the last\n" +
			"check is not necessarily this tool's doing. ERROR 130 here means the plan\n" +
			"has no API add-on at all, which no `--limit` can work around.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			base := strings.TrimRight(s.unitsBaseURL(), "/") + unitsPath
			body, err := s.getRaw(cmd.Context(), base, url.Values{}, key)
			if err != nil {
				return err
			}
			trimmed := strings.TrimSpace(string(body))
			if code, message, ok := parseSemrushError(trimmed); ok {
				return classifyReportError(code, message)
			}
			// countapiunits returns a bare integer, sometimes with thousands
			// separators (e.g. "1,000").
			digits := strings.ReplaceAll(trimmed, ",", "")
			units, convErr := strconv.ParseInt(digits, 10, 64)
			if convErr != nil {
				return &apiError{msg: "semrush: unexpected API-units response: " + trimmed}
			}
			return s.emitJSON(map[string]any{"api_units_remaining": units})
		},
	}
}
