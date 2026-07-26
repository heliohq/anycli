package beehiiv

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newSubscriptionCmd(token string) *cobra.Command {
	cmd := newGroupCmd("subscription", "Subscribers (get-by-email, list, create, update)")
	cmd.AddCommand(
		s.newSubscriptionGetByEmailCmd(token),
		s.newSubscriptionListCmd(token),
		s.newSubscriptionCreateCmd(token),
		s.newSubscriptionUpdateCmd(token),
	)
	return cmd
}

func (s *Service) newSubscriptionGetByEmailCmd(token string) *cobra.Command {
	var expand []string
	cmd := &cobra.Command{
		Use:   "get-by-email <email>",
		Short: "Look up a subscriber by email (GET /publications/{pub}/subscriptions/by_email/{email})",
		Long: "The exact-match lookup, and the address is URL-encoded for you, so a\n" +
			"`+` tag or an unusual character needs no escaping.\n" +
			"`subscription list --email` is the partial-match search instead. Run\n" +
			"this with `--expand custom_fields` before an update: the returned\n" +
			"`sub_…` id is what `subscription update` takes, and the current custom\n" +
			"field values are what a careless update would otherwise overwrite. An\n" +
			"unknown address comes back as a 404, not an empty result.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			pubID, err := cmd.Flags().GetString("publication-id")
			if err != nil {
				return err
			}
			pub, err := requirePublicationID(pubID)
			if err != nil {
				return err
			}
			q := url.Values{}
			addExpand(q, expand)
			// beehiiv requires the email path segment to be URL-encoded
			// (e.g. @ -> %40). QueryEscape encodes @, + and other reserved
			// characters; emails carry no spaces, so its space->'+' quirk
			// never applies.
			email := url.QueryEscape(args[0])
			resp, err := s.call(cmd.Context(), token, http.MethodGet,
				"/publications/"+pub+"/subscriptions/by_email/"+email, q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	addPublicationFlag(cmd)
	cmd.Flags().StringSliceVar(&expand, "expand", nil, "extra fields: stats, custom_fields, referrals, tags, … (repeatable)")
	return cmd
}

func (s *Service) newSubscriptionListCmd(token string) *cobra.Command {
	var (
		expand                                                       []string
		status, tier, email, orderBy, direction, cursor, page, limit string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List subscribers (GET /publications/{pub}/subscriptions)",
		Long: "`--status` filters by lifecycle (`active`, `inactive`, `pending`,\n" +
			"`validating`, `invalid`) and `--tier` by `free|premium|all`.\n" +
			"`--email` here is a PARTIAL match, which makes it a search rather than\n" +
			"a lookup — use `subscription get-by-email` when the address is known\n" +
			"exactly. `--expand stats`, `custom_fields` or `tags` add fields that\n" +
			"are otherwise absent. `--limit` is 1-100; continue with `--cursor` or\n" +
			"`--page`.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pubID, err := cmd.Flags().GetString("publication-id")
			if err != nil {
				return err
			}
			pub, err := requirePublicationID(pubID)
			if err != nil {
				return err
			}
			q := url.Values{}
			addExpand(q, expand)
			setIfNonEmpty(q, "status", status)
			setIfNonEmpty(q, "tier", tier)
			setIfNonEmpty(q, "email", email)
			setIfNonEmpty(q, "order_by", orderBy)
			setIfNonEmpty(q, "direction", direction)
			setIfNonEmpty(q, "cursor", cursor)
			setIfNonEmpty(q, "page", page)
			setIfNonEmpty(q, "limit", limit)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/publications/"+pub+"/subscriptions", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	addPublicationFlag(cmd)
	cmd.Flags().StringSliceVar(&expand, "expand", nil, "extra fields: stats, custom_fields, tags, … (repeatable)")
	cmd.Flags().StringVar(&status, "status", "", "filter by status: validating|invalid|pending|active|inactive|…")
	cmd.Flags().StringVar(&tier, "tier", "", "filter by tier: free|premium|all")
	cmd.Flags().StringVar(&email, "email", "", "filter by (partial) email")
	cmd.Flags().StringVar(&orderBy, "order-by", "", "sort field: created|email")
	cmd.Flags().StringVar(&direction, "direction", "", "sort direction: asc|desc")
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor")
	cmd.Flags().StringVar(&page, "page", "", "page number")
	cmd.Flags().StringVar(&limit, "limit", "", "page size (1-100)")
	return cmd
}

func (s *Service) newSubscriptionCreateCmd(token string) *cobra.Command {
	var email, data string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Add a subscriber (POST /publications/{pub}/subscriptions)",
		Long: "`--email` is required and always wins over an `email` key inside\n" +
			"`--data`. Everything else rides `--data`, a JSON object validated\n" +
			"before the call: `reactivate_existing` to bring a previously\n" +
			"unsubscribed reader back instead of failing on the duplicate,\n" +
			"`send_welcome_email` — which sends a real email to a real person —\n" +
			"`tier`, the `utm_*` attribution fields, `automation_ids` to drop the\n" +
			"subscriber straight into an automation, and `custom_fields` as\n" +
			"`[{\"name\":…,\"value\":…}]` with names from `custom-field list`.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pubID, err := cmd.Flags().GetString("publication-id")
			if err != nil {
				return err
			}
			pub, err := requirePublicationID(pubID)
			if err != nil {
				return err
			}
			body := map[string]any{}
			if data != "" {
				body, err = decodeJSONObject("data", data)
				if err != nil {
					return err
				}
			}
			// --email is authoritative over any email carried in --data.
			body["email"] = email
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/publications/"+pub+"/subscriptions", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	addPublicationFlag(cmd)
	cmd.Flags().StringVar(&email, "email", "", "subscriber email address (required)")
	_ = cmd.MarkFlagRequired("email")
	cmd.Flags().StringVar(&data, "data", "", "raw JSON object of optional fields (reactivate_existing, tier, custom_fields, utm_*, automation_ids, …)")
	return cmd
}

func (s *Service) newSubscriptionUpdateCmd(token string) *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:   "update <subscriptionId>",
		Short: "Update a subscriber (PUT /publications/{pub}/subscriptions/{subscriptionId})",
		Long: "The positional argument is the `sub_…` id from\n" +
			"`subscription get-by-email` or `subscription list`; an email address is\n" +
			"not accepted here. `--data` carries only the fields to CHANGE —\n" +
			"`{\"tier\":\"premium\"}`, `{\"email\":\"new@addr\"}`,\n" +
			"`{\"custom_fields\":[{\"name\":\"Plan\",\"value\":\"pro\",\"delete\":false}]}` —\n" +
			"and anything omitted is left as it stands. `{\"unsubscribe\":true}` is\n" +
			"the removal path, since no delete command exists; it stops delivery to\n" +
			"a real reader immediately.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			pubID, err := cmd.Flags().GetString("publication-id")
			if err != nil {
				return err
			}
			pub, err := requirePublicationID(pubID)
			if err != nil {
				return err
			}
			if data == "" {
				return &usageError{msg: "--data is required (a JSON object of fields to update, e.g. {\"tier\":\"premium\"})"}
			}
			body, err := decodeJSONObject("data", data)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPut,
				"/publications/"+pub+"/subscriptions/"+url.PathEscape(args[0]), nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	addPublicationFlag(cmd)
	cmd.Flags().StringVar(&data, "data", "", "raw JSON object of fields to update (tier, email, unsubscribe, custom_fields, …)")
	return cmd
}
