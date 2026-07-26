package courier

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newAutomationInvokeCmd builds `automation invoke` — POST /automations/invoke,
// triggering an ad-hoc automation run. The automation definition is a required
// JSON object; recipient / template / brand / data / profile are optional
// top-level fields on the invoke body.
func (s *Service) newAutomationInvokeCmd(key string) *cobra.Command {
	var automation, recipient, template, brand, data, profile string
	cmd := &cobra.Command{
		Use:   "invoke",
		Short: "Trigger an ad-hoc automation run",
		Long: "--automation is a required JSON object holding the whole definition — a\n" +
			"`steps` array of actions such as send, delay, update-profile and cancel —\n" +
			"which runs immediately and is never saved as a named automation.\n" +
			"--recipient, --template, --brand, --data and --profile are optional\n" +
			"top-level fields the steps interpolate. Reach for this when the sequence\n" +
			"needs delays or conditions; one immediate notification is cheaper through\n" +
			"`send`.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if automation == "" {
				return &usageError{msg: "courier automation invoke: --automation (JSON object) is required"}
			}
			auto, err := decodeJSONObjectFlag("automation", automation)
			if err != nil {
				return err
			}
			payload := map[string]any{"automation": auto}
			if recipient != "" {
				payload["recipient"] = recipient
			}
			if template != "" {
				payload["template"] = template
			}
			if brand != "" {
				payload["brand"] = brand
			}
			if data != "" {
				d, err := decodeJSONObjectFlag("data", data)
				if err != nil {
					return err
				}
				payload["data"] = d
			}
			if profile != "" {
				p, err := decodeJSONObjectFlag("profile", profile)
				if err != nil {
					return err
				}
				payload["profile"] = p
			}
			out, err := s.call(cmd.Context(), key, http.MethodPost, "/automations/invoke", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(out)
		},
	}
	pf := cmd.Flags()
	pf.StringVar(&automation, "automation", "", "automation definition as a JSON object (required)")
	pf.StringVar(&recipient, "recipient", "", "recipient user id")
	pf.StringVar(&template, "template", "", "template id")
	pf.StringVar(&brand, "brand", "", "brand id")
	pf.StringVar(&data, "data", "", "template variables as a JSON object")
	pf.StringVar(&profile, "profile", "", "recipient profile as a JSON object")
	return cmd
}
