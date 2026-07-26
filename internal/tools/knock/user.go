package knock

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newUserCmd groups the user (recipient) verbs: identify/upsert, read, list,
// delete, merge, plus per-user preferences and channel data.
func (s *Service) newUserCmd(key string) *cobra.Command {
	group := newGroupCmd("user", "Manage recipients (users), preferences, and channel data")
	group.AddCommand(
		s.newUserIdentifyCmd(key),
		s.newUserGetCmd(key),
		s.newUserListCmd(key),
		s.newUserDeleteCmd(key),
		s.newUserMergeCmd(key),
		s.newUserGetPreferencesCmd(key),
		s.newUserSetPreferencesCmd(key),
		s.newUserGetChannelDataCmd(key),
		s.newUserSetChannelDataCmd(key),
		s.newUserDeleteChannelDataCmd(key),
	)
	return group
}

func (s *Service) newUserIdentifyCmd(key string) *cobra.Command {
	var (
		id   string
		data string
	)
	cmd := &cobra.Command{
		Use:   "identify",
		Short: "Identify (create or update) a user",
		Long: "An upsert keyed on --id, and that id is yours to choose — it is what every\n" +
			"--recipient elsewhere in the tool refers to. --data REPLACES the user's\n" +
			"properties rather than merging into them, so send the full set each time;\n" +
			"omitting `email` on a second call drops the address. Those properties are\n" +
			"what the workflow templates render and what channel routing reads (email,\n" +
			"phone_number, timezone).",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID("id", id); err != nil {
				return err
			}
			body := map[string]any{}
			if data != "" {
				parsed, decErr := decodeJSONFlag("data", data)
				if decErr != nil {
					return decErr
				}
				obj, ok := parsed.(map[string]any)
				if !ok {
					return &usageError{msg: "--data must be a JSON object"}
				}
				body = obj
			}
			return s.callEmit(cmd.Context(), key, http.MethodPut, "/users/"+url.PathEscape(id), nil, body, nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "user id (required)")
	cmd.Flags().StringVar(&data, "data", "", "user properties as a JSON object (name, email, phone_number, …)")
	return cmd
}

func (s *Service) newUserGetCmd(key string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a user",
		Long: "Requires --id. Returns the user's stored properties only — preferences and\n" +
			"channel data live behind `user get-preferences` and `user\n" +
			"get-channel-data`, so a user who looks reachable here may still be opted\n" +
			"out or have no device registered.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID("id", id); err != nil {
				return err
			}
			return s.callEmit(cmd.Context(), key, http.MethodGet, "/users/"+url.PathEscape(id), nil, nil, nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "user id (required)")
	return cmd
}

func (s *Service) newUserListCmd(key string) *cobra.Command {
	var (
		pageSize int
		after    string
		before   string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List users",
		Long: "Every user in the connected environment, paged with --page-size (Knock's\n" +
			"default is 50) and --after. There is no search or filter here, so finding\n" +
			"somebody by email means scanning — which is why the id passed to `user\n" +
			"identify` should be one the calling system can reproduce.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			addPaging(q, pageSize, after, before)
			return s.callEmit(cmd.Context(), key, http.MethodGet, "/users", q, nil, nil)
		},
	}
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "page size (Knock default 50)")
	cmd.Flags().StringVar(&after, "after", "", "pagination cursor (next page)")
	cmd.Flags().StringVar(&before, "before", "", "pagination cursor (previous page)")
	return cmd
}

func (s *Service) newUserDeleteCmd(key string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a user",
		Long: "Requires --id, and takes the user's preferences and channel data with them,\n" +
			"so any later workflow addressed to that id reaches nobody. Nothing here\n" +
			"restores them. To fold a duplicate into a survivor use `user merge`, which\n" +
			"keeps the history instead of discarding it.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID("id", id); err != nil {
				return err
			}
			return s.callEmit(cmd.Context(), key, http.MethodDelete, "/users/"+url.PathEscape(id), nil, nil, nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "user id (required)")
	return cmd
}

func (s *Service) newUserMergeCmd(key string) *cobra.Command {
	var (
		id     string
		fromID string
	)
	cmd := &cobra.Command{
		Use:   "merge",
		Short: "Merge one user into another (from-id merged into id)",
		Long: "--id is the SURVIVOR and --from-id is the user merged away and removed.\n" +
			"Reversing the two destroys the record that was meant to be kept, and\n" +
			"nothing here undoes it. This is the right command when a duplicate has to\n" +
			"disappear but its notification history should not.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID("id", id); err != nil {
				return err
			}
			if err := requireID("from-id", fromID); err != nil {
				return err
			}
			body := map[string]any{"from_user_id": fromID}
			return s.callEmit(cmd.Context(), key, http.MethodPost, "/users/"+url.PathEscape(id)+"/merge", nil, body, nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "destination user id the other is merged into (required)")
	cmd.Flags().StringVar(&fromID, "from-id", "", "source user id merged away (required)")
	return cmd
}

func (s *Service) newUserGetPreferencesCmd(key string) *cobra.Command {
	var (
		id  string
		set string
	)
	cmd := &cobra.Command{
		Use:   "get-preferences",
		Short: "Get a user's preference sets (or one set with --set)",
		Long: "Requires --id. Omitting --set lists every preference set the user has;\n" +
			"passing --set returns just that one, usually `default`. Preferences are\n" +
			"opt-outs, so a channel or workflow disabled here suppresses delivery even\n" +
			"though `workflow trigger` still succeeds and returns a run id — read this\n" +
			"before concluding that a missing notification was a bug.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID("id", id); err != nil {
				return err
			}
			path := "/users/" + url.PathEscape(id) + "/preferences"
			if set != "" {
				path += "/" + url.PathEscape(set)
			}
			return s.callEmit(cmd.Context(), key, http.MethodGet, path, nil, nil, nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "user id (required)")
	cmd.Flags().StringVar(&set, "set", "", "preference set id (omit to list all sets)")
	return cmd
}

func (s *Service) newUserSetPreferencesCmd(key string) *cobra.Command {
	var (
		id   string
		set  string
		data string
	)
	cmd := &cobra.Command{
		Use:   "set-preferences",
		Short: "Replace a user's preference set",
		Long: "--id, --set and --data are all required. This REPLACES the named preference\n" +
			"set outright — anything absent from --data is gone, including opt-outs the\n" +
			"user set themselves — so read the current set with `user get-preferences`\n" +
			"and send it back modified. `default` is the usual set id.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID("id", id); err != nil {
				return err
			}
			if err := requireID("set", set); err != nil {
				return err
			}
			if err := requireID("data", data); err != nil {
				return err
			}
			parsed, decErr := decodeJSONFlag("data", data)
			if decErr != nil {
				return decErr
			}
			return s.callEmit(cmd.Context(), key, http.MethodPut, "/users/"+url.PathEscape(id)+"/preferences/"+url.PathEscape(set), nil, parsed, nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "user id (required)")
	cmd.Flags().StringVar(&set, "set", "", "preference set id, e.g. default (required)")
	cmd.Flags().StringVar(&data, "data", "", "preference set as a JSON object (required)")
	return cmd
}

func (s *Service) newUserGetChannelDataCmd(key string) *cobra.Command {
	var (
		id        string
		channelID string
	)
	cmd := &cobra.Command{
		Use:   "get-channel-data",
		Short: "Get a user's channel data for a channel",
		Long: "--id and --channel-id are both required, and the channel id comes from the\n" +
			"Knock dashboard — this tool has no channel listing to look it up in.\n" +
			"Returns the per-channel routing data, such as the push tokens registered\n" +
			"for that user.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID("id", id); err != nil {
				return err
			}
			if err := requireID("channel-id", channelID); err != nil {
				return err
			}
			return s.callEmit(cmd.Context(), key, http.MethodGet, "/users/"+url.PathEscape(id)+"/channel_data/"+url.PathEscape(channelID), nil, nil, nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "user id (required)")
	cmd.Flags().StringVar(&channelID, "channel-id", "", "channel id (required)")
	return cmd
}

func (s *Service) newUserSetChannelDataCmd(key string) *cobra.Command {
	var (
		id        string
		channelID string
		data      string
	)
	cmd := &cobra.Command{
		Use:   "set-channel-data",
		Short: "Set a user's channel data for a channel",
		Long: "--id, --channel-id and --data are all required. --data takes the channel's\n" +
			"own shape — `{\"tokens\":[…]}` for push, a connection object for Slack — and\n" +
			"it REPLACES what was stored, so a device registered earlier disappears\n" +
			"unless its token is re-sent. A user with no channel data for a channel is\n" +
			"simply unreachable on it, with no error at trigger time.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID("id", id); err != nil {
				return err
			}
			if err := requireID("channel-id", channelID); err != nil {
				return err
			}
			if err := requireID("data", data); err != nil {
				return err
			}
			parsed, decErr := decodeJSONFlag("data", data)
			if decErr != nil {
				return decErr
			}
			body := map[string]any{"data": parsed}
			return s.callEmit(cmd.Context(), key, http.MethodPut, "/users/"+url.PathEscape(id)+"/channel_data/"+url.PathEscape(channelID), nil, body, nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "user id (required)")
	cmd.Flags().StringVar(&channelID, "channel-id", "", "channel id (required)")
	cmd.Flags().StringVar(&data, "data", "", "channel data as a JSON object, e.g. {\"tokens\":[...]} (required)")
	return cmd
}

func (s *Service) newUserDeleteChannelDataCmd(key string) *cobra.Command {
	var (
		id        string
		channelID string
	)
	cmd := &cobra.Command{
		Use:   "delete-channel-data",
		Short: "Delete a user's channel data for a channel",
		Long: "--id and --channel-id are both required. This unregisters the user from\n" +
			"that one channel — the tokens or connection go away — while the user and\n" +
			"their other channels stay intact. It is how a signed-out device stops\n" +
			"receiving notifications.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID("id", id); err != nil {
				return err
			}
			if err := requireID("channel-id", channelID); err != nil {
				return err
			}
			return s.callEmit(cmd.Context(), key, http.MethodDelete, "/users/"+url.PathEscape(id)+"/channel_data/"+url.PathEscape(channelID), nil, nil, nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "user id (required)")
	cmd.Flags().StringVar(&channelID, "channel-id", "", "channel id (required)")
	return cmd
}
