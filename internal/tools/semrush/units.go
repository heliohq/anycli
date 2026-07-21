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
		Args:  cobra.NoArgs,
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
