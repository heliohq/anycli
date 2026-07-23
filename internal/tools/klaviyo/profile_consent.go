package klaviyo

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newProfileConsentCmds builds the four single-profile consent operations that
// wrap Klaviyo's bulk-job API: subscribe/unsubscribe (subscription jobs) and
// suppress/unsuppress (suppression jobs). Each is a convenience over the bulk
// endpoint for one profile, returning the 202 job receipt verbatim. --data
// overrides the constructed body for full control.
func (s *Service) newProfileConsentCmds(token string) []*cobra.Command {
	return []*cobra.Command{
		s.newSubscriptionJobCmd(token, "subscribe",
			"Subscribe a profile to email/SMS marketing (POST /profile-subscription-bulk-create-jobs)",
			"/profile-subscription-bulk-create-jobs", "profile-subscription-bulk-create-job", true),
		s.newSubscriptionJobCmd(token, "unsubscribe",
			"Unsubscribe a profile (POST /profile-subscription-bulk-delete-jobs)",
			"/profile-subscription-bulk-delete-jobs", "profile-subscription-bulk-delete-job", false),
		s.newSuppressionJobCmd(token, "suppress",
			"Suppress a profile from email marketing (POST /profile-suppression-bulk-create-jobs)",
			"/profile-suppression-bulk-create-jobs", "profile-suppression-bulk-create-job"),
		s.newSuppressionJobCmd(token, "unsuppress",
			"Remove a profile from the suppression list (POST /profile-suppression-bulk-delete-jobs)",
			"/profile-suppression-bulk-delete-jobs", "profile-suppression-bulk-delete-job"),
	}
}

// newSubscriptionJobCmd builds a subscribe/unsubscribe command. withConsent
// adds the per-channel SUBSCRIBED consent block required by the create job;
// the delete job omits it. --list-id sets the optional list relationship.
func (s *Service) newSubscriptionJobCmd(token, use, short, path, jobType string, withConsent bool) *cobra.Command {
	var email, phone, listID, channel, data string
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := subscriptionJobBody(jobType, email, phone, listID, channel, withConsent, data)
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, path, nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "profile email")
	cmd.Flags().StringVar(&phone, "phone", "", "profile phone_number (E.164), required for SMS")
	cmd.Flags().StringVar(&listID, "list-id", "", "list to subscribe/unsubscribe against (optional)")
	if withConsent {
		cmd.Flags().StringVar(&channel, "channel", "email", "consent channel: email|sms")
	}
	cmd.Flags().StringVar(&data, "data", "", "raw JSON:API request body (overrides the shorthand)")
	return cmd
}

// newSuppressionJobCmd builds a suppress/unsuppress command. Suppression jobs
// take only the profile identifier (no list, no consent channel).
func (s *Service) newSuppressionJobCmd(token, use, short, path, jobType string) *cobra.Command {
	var email, data string
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := suppressionJobBody(jobType, email, data)
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, path, nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "profile email to suppress/unsuppress")
	cmd.Flags().StringVar(&data, "data", "", "raw JSON:API request body (overrides the shorthand)")
	return cmd
}

// subscriptionJobBody builds the subscription bulk-job envelope for one
// profile. --data wins verbatim. The create job adds the channel consent block;
// the delete job carries only the identifier. A list relationship is attached
// when --list-id is set.
func subscriptionJobBody(jobType, email, phone, listID, channel string, withConsent bool, data string) (any, error) {
	if data != "" {
		return parseDataFlag(data)
	}
	if email == "" && phone == "" {
		return nil, &usageError{msg: "provide --email or --phone, or --data"}
	}
	profileAttrs := compactAttrs(map[string]string{"email": email, "phone_number": phone})
	if withConsent {
		consent, err := consentBlock(channel, phone)
		if err != nil {
			return nil, err
		}
		profileAttrs["subscriptions"] = consent
	}
	attrs := map[string]any{
		"profiles": map[string]any{
			"data": []map[string]any{{"type": "profile", "attributes": profileAttrs}},
		},
	}
	var relationships map[string]any
	if listID != "" {
		relationships = map[string]any{"list": singleRelationship("list", listID)}
	}
	return resourceBody(jobType, "", attrs, relationships), nil
}

// consentBlock builds the per-channel marketing SUBSCRIBED consent object. SMS
// consent requires a phone number.
func consentBlock(channel, phone string) (map[string]any, error) {
	switch channel {
	case "", "email":
		return map[string]any{"email": map[string]any{"marketing": map[string]any{"consent": "SUBSCRIBED"}}}, nil
	case "sms":
		if phone == "" {
			return nil, &usageError{msg: "--channel sms requires --phone"}
		}
		return map[string]any{"sms": map[string]any{"marketing": map[string]any{"consent": "SUBSCRIBED"}}}, nil
	default:
		return nil, &usageError{msg: "--channel must be email or sms, got " + channel}
	}
}

// suppressionJobBody builds the suppression bulk-job envelope for one profile.
func suppressionJobBody(jobType, email, data string) (any, error) {
	if data != "" {
		return parseDataFlag(data)
	}
	if email == "" {
		return nil, &usageError{msg: "provide --email, or --data"}
	}
	attrs := map[string]any{
		"profiles": map[string]any{
			"data": []map[string]any{{"type": "profile", "attributes": map[string]any{"email": email}}},
		},
	}
	return resourceBody(jobType, "", attrs, nil), nil
}
