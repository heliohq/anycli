package delighted

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newAutopilotCmd wires the Autopilot resource: membership list/add/remove and
// config read, each scoped to a required --platform (email|sms) path segment.
func (s *Service) newAutopilotCmd(key string) *cobra.Command {
	cmd := &cobra.Command{Use: "autopilot", Short: "Autopilot enrollment (email/sms)"}
	cmd.AddCommand(
		s.newAutopilotMembershipsCmd(key),
		s.newAutopilotConfigCmd(key),
	)
	return cmd
}

func (s *Service) newAutopilotMembershipsCmd(key string) *cobra.Command {
	cmd := &cobra.Command{Use: "memberships", Short: "Autopilot memberships"}
	cmd.AddCommand(
		s.newAutopilotMembershipListCmd(key),
		s.newAutopilotMembershipAddCmd(key),
		s.newAutopilotMembershipRemoveCmd(key),
	)
	return cmd
}

// platformFlag registers the required --platform enum flag and validates it.
func platformFlag(cmd *cobra.Command, platform *string) {
	cmd.Flags().StringVar(platform, "platform", "", "autopilot platform: email or sms")
	_ = cmd.MarkFlagRequired("platform")
}

func validatePlatform(platform string) error {
	if platform != "email" && platform != "sms" {
		return fmt.Errorf("delighted: --platform must be email or sms, got %q", platform)
	}
	return nil
}

func (s *Service) newAutopilotMembershipListCmd(key string) *cobra.Command {
	var platform, personEmail string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List autopilot memberships (GET /autopilot/{platform}/memberships.json)",
		Long: "Autopilot keeps two entirely separate rosters, so `--platform email` and\n" +
			"`--platform sms` return different people and neither implies the other;\n" +
			"the flag is required and rejected unless it is exactly email or sms.\n" +
			"`--person-email` narrows to one person, which is the cheap way to answer\n" +
			"\"is this customer enrolled\" without paging the roster.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validatePlatform(platform); err != nil {
				return err
			}
			q := url.Values{}
			setIfNonEmpty(q, "person_email", personEmail)
			path := "/autopilot/" + platform + "/memberships.json"
			resp, err := s.call(cmd.Context(), key, http.MethodGet, path, q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	platformFlag(cmd, &platform)
	cmd.Flags().StringVar(&personEmail, "person-email", "", "filter to one person's membership")
	return cmd
}

func (s *Service) newAutopilotMembershipAddCmd(key string) *cobra.Command {
	var platform, personEmail, name, properties string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a person to autopilot (POST /autopilot/{platform}/memberships.json)",
		Long: "Enrolls the person in a RECURRING survey schedule on the given\n" +
			"`--platform`, so unlike `people send` this keeps mailing them on the\n" +
			"project's cadence until they are removed. Enrollment is per platform:\n" +
			"adding to email leaves sms untouched. `--properties-json` sets the custom\n" +
			"attributes the schedule and reports segment on. Someone on the\n" +
			"unsubscribe list stays suppressed regardless.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validatePlatform(platform); err != nil {
				return err
			}
			person := map[string]any{"email": personEmail}
			if name != "" {
				person["name"] = name
			}
			if properties != "" {
				v, err := decodeJSONFlag("properties-json", properties)
				if err != nil {
					return err
				}
				person["properties"] = v
			}
			payload := map[string]any{"person": person}
			path := "/autopilot/" + platform + "/memberships.json"
			resp, err := s.call(cmd.Context(), key, http.MethodPost, path, nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	platformFlag(cmd, &platform)
	cmd.Flags().StringVar(&personEmail, "person-email", "", "email of the person to enroll")
	cmd.Flags().StringVar(&name, "name", "", "person's name")
	cmd.Flags().StringVar(&properties, "properties-json", "", "custom person properties as a raw JSON object")
	_ = cmd.MarkFlagRequired("person-email")
	return cmd
}

func (s *Service) newAutopilotMembershipRemoveCmd(key string) *cobra.Command {
	var platform, personEmail string
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove a person from autopilot (DELETE /autopilot/{platform}/memberships.json)",
		Long: "Ends the recurring enrollment for `--person-email` on one `--platform`\n" +
			"only — someone enrolled in both email and sms needs two calls. The person,\n" +
			"their properties and their past responses all survive; this stops future\n" +
			"scheduled sends, it does not unsubscribe them (`unsubscribes add`) or\n" +
			"cancel a survey already queued (`people cancel-pending`).",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validatePlatform(platform); err != nil {
				return err
			}
			q := url.Values{}
			q.Set("person_email", personEmail)
			path := "/autopilot/" + platform + "/memberships.json"
			resp, err := s.call(cmd.Context(), key, http.MethodDelete, path, q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	platformFlag(cmd, &platform)
	cmd.Flags().StringVar(&personEmail, "person-email", "", "email of the person to remove")
	_ = cmd.MarkFlagRequired("person-email")
	return cmd
}

func (s *Service) newAutopilotConfigCmd(key string) *cobra.Command {
	var platform string
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Autopilot configuration",
	}
	get := &cobra.Command{
		Use:   "get",
		Short: "Read autopilot configuration (GET /autopilot/{platform}.json)",
		Long: "Reads one platform's schedule settings — whether Autopilot is running and\n" +
			"how often it re-surveys the same person. That interval explains why an\n" +
			"`autopilot memberships add` produces no immediate survey. Read-only:\n" +
			"the configuration itself is changed in the Delighted UI, and `--platform`\n" +
			"(email or sms) is required.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validatePlatform(platform); err != nil {
				return err
			}
			path := "/autopilot/" + platform + ".json"
			resp, err := s.call(cmd.Context(), key, http.MethodGet, path, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	platformFlag(get, &platform)
	cmd.AddCommand(get)
	return cmd
}
