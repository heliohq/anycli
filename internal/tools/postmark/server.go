package postmark

import (
	"encoding/json"
	"net/http"

	"github.com/spf13/cobra"
)

// serverView is the REDACTED projection of GET /server. GET /server echoes the
// caller's own Server API Token back in an ApiTokens array; that field must
// never reach stdout, so this view is an allowlist decoded from the response —
// it has no ApiTokens field by construction, so no future secret-bearing field
// Postmark adds can leak through `server get`.
type serverView struct {
	ID               int    `json:"ID"`
	Name             string `json:"Name"`
	Color            string `json:"Color"`
	ServerLink       string `json:"ServerLink"`
	DeliveryType     string `json:"DeliveryType"`
	SMTPAPIActivated bool   `json:"SmtpApiActivated"`
	InboundAddress   string `json:"InboundAddress"`
	InboundDomain    string `json:"InboundDomain"`
	TrackOpens       bool   `json:"TrackOpens"`
	TrackLinks       string `json:"TrackLinks"`
}

func (s *Service) newServerCmd(token string) *cobra.Command {
	group := newGroupCmd("server", "Inspect the connected server")
	group.AddCommand(&cobra.Command{
		Use:   "get",
		Short: "Show current server metadata, redacting API tokens (GET /server)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			raw, err := s.call(cmd.Context(), token, http.MethodGet, "/server", nil, nil)
			if err != nil {
				return err
			}
			var view serverView
			if err := json.Unmarshal(raw, &view); err != nil {
				return &apiError{msg: "postmark: decode server response: " + err.Error(), err: err}
			}
			return s.emitValue(view)
		},
	})
	return group
}
