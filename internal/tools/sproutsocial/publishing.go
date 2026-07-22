package sproutsocial

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newPublishingCmd is the publishing group. Sprout's public API creates draft
// posts only (is_draft is always true); scheduling/approval happen in-app.
func (s *Service) newPublishingCmd(token string) *cobra.Command {
	cmd := newGroupCmd("publishing", "Draft posts (POST /v1/{cid}/publishing/posts)")
	cmd.AddCommand(
		s.newPublishingCreateCmd(token),
		s.newPublishingGetCmd(token),
	)
	return cmd
}

// newPublishingCreateCmd creates a draft post. The assembled body always sets
// is_draft:true (the only mode the API supports) and requires a group id plus
// at least one profile id; --body sends a verbatim JSON object instead, for
// media, delivery scheduling, tags, or any field not surfaced as a flag.
func (s *Service) newPublishingCreateCmd(token string) *cobra.Command {
	var groupID, text, body string
	var profileIDs []string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a draft post (POST /v1/{cid}/publishing/posts)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cid, err := resolveCID(cmd)
			if err != nil {
				return err
			}
			payload, err := buildPublishingBody(groupID, text, body, profileIDs)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/v1/"+cid+"/publishing/posts", payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&groupID, "group-id", "", "publishing group id (required unless --body)")
	cmd.Flags().StringArrayVar(&profileIDs, "profile-id", nil, "target customer profile id (repeatable; required unless --body)")
	cmd.Flags().StringVar(&text, "text", "", "post text")
	cmd.Flags().StringVar(&body, "body", "", "raw JSON request body (verbatim passthrough; overrides the flags above)")
	return cmd
}

// buildPublishingBody assembles the draft-post request. With --body set it
// returns that JSON object verbatim; otherwise it requires a group id and at
// least one profile id and always forces is_draft:true. Numeric ids are coerced
// to numbers (Sprout ids are numeric) and left as strings otherwise.
func buildPublishingBody(groupID, text, body string, profileIDs []string) (any, error) {
	if body != "" {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(body), &raw); err != nil {
			return nil, &usageError{msg: fmt.Sprintf("--body is not a valid JSON object: %v", err)}
		}
		return raw, nil
	}
	if groupID == "" {
		return nil, &usageError{msg: "publishing create requires --group-id (or --body)"}
	}
	if len(profileIDs) == 0 {
		return nil, &usageError{msg: "publishing create requires at least one --profile-id (or --body)"}
	}
	profiles := make([]any, len(profileIDs))
	for i, p := range profileIDs {
		profiles[i] = coerceID(p)
	}
	m := map[string]any{
		"group_id":             coerceID(groupID),
		"customer_profile_ids": profiles,
		"is_draft":             true,
	}
	if text != "" {
		m["text"] = text
	}
	return m, nil
}

// coerceID returns v as an int64 when it is a base-10 integer, otherwise the
// original string. Sprout resource ids are numeric, but keeping a string
// fallback avoids rejecting an unexpected id shape outright.
func coerceID(v string) any {
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		return n
	}
	return v
}

// newPublishingGetCmd retrieves one draft post by its publishing_post_id.
func (s *Service) newPublishingGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <publishing_post_id>",
		Short: "Get a draft post (GET /v1/{cid}/publishing/posts/{id})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cid, err := resolveCID(cmd)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v1/"+cid+"/publishing/posts/"+url.PathEscape(args[0]), nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}
