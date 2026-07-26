package instantly

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newLeadListCmd(token string) *cobra.Command {
	cmd := newGroupCmd("lead-list", "Lead lists (staging leads before campaign assignment)")
	cmd.AddCommand(
		s.newLeadListListCmd(token),
		s.newLeadListGetCmd(token),
		s.newLeadListCreateCmd(token),
		s.newLeadListUpdateCmd(token),
		s.newLeadListDeleteCmd(token),
		s.newLeadListVerificationStatsCmd(token),
	)
	return cmd
}

func (s *Service) newLeadListListCmd(token string) *cobra.Command {
	var page pageFlags
	var search string
	cmd := &cobra.Command{
		Use:         "list",
		Annotations: readOnly,
		Short:       "List lead lists (GET /lead-lists)",
		Long: "--search matches the list NAME as a substring; cursor-paged with --limit\n" +
			"and --starting-after. The ids here are what --list-id takes on `lead\n" +
			"create`, `lead add`, `lead move` and `lead list`.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			page.applyQuery(q)
			setIfChanged(cmd, q, "search", "search", search)
			return s.get(cmd, token, "/lead-lists", q)
		},
	}
	registerPageFlags(cmd, &page)
	cmd.Flags().StringVar(&search, "search", "", "filter by name substring")
	return cmd
}

func (s *Service) newLeadListGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         "get",
		Annotations: readOnly,
		Short:       "Get a lead list (GET /lead-lists/{id})",
		Long: "--id is required and returns the list's own record. Its leads come from\n" +
			"`lead list --list-id <id>` and its address quality from `lead-list\n" +
			"verification-stats` — neither is included here.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.get(cmd, token, "/lead-lists/"+url.PathEscape(id), nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "lead-list id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newLeadListCreateCmd(token string) *cobra.Command {
	var data, name string
	cmd := &cobra.Command{
		Use:         "create",
		Annotations: writeAction,
		Short:       "Create a lead list (POST /lead-lists)",
		Long: "--name is the convenience flag over the raw --data body. A lead list is a\n" +
			"staging area: nothing emails the leads in it until they are added or\n" +
			"moved into a campaign, which makes it the safe place to assemble and\n" +
			"verify an audience before any sending starts.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := decodeDataFlag(data)
			if err != nil {
				return err
			}
			setBodyIfChanged(cmd, body, "name", "name", name)
			return s.send(cmd, token, http.MethodPost, "/lead-lists", body)
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "raw JSON lead-list body (merged; flags override)")
	cmd.Flags().StringVar(&name, "name", "", "lead-list name")
	return cmd
}

func (s *Service) newLeadListUpdateCmd(token string) *cobra.Command {
	var id, data string
	cmd := &cobra.Command{
		Use:         "update",
		Annotations: writeAction,
		Short:       "Update a lead list (PATCH /lead-lists/{id}). --data is the raw JSON body",
		Long: "--id is required and --data is the raw patch body; there are no per-field\n" +
			"flags at all here, so read the current shape with `lead-list get` first.\n" +
			"This edits the list itself, never its membership — leads move with `lead\n" +
			"add` and `lead move`.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := decodeDataFlag(data)
			if err != nil {
				return err
			}
			return s.send(cmd, token, http.MethodPatch, "/lead-lists/"+url.PathEscape(id), body)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "lead-list id")
	cmd.Flags().StringVar(&data, "data", "", "raw JSON patch body")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newLeadListDeleteCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         "delete",
		Annotations: writeAction,
		Short:       "Delete a lead list (DELETE /lead-lists/{id})",
		Long: "--id is required. Nothing in this tool restores a deleted lead list and\n" +
			"there is no archive state, so re-creating and re-populating is the only\n" +
			"recovery. Read the membership with `lead list --list-id <id>` first if it\n" +
			"still matters.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.send(cmd, token, http.MethodDelete, "/lead-lists/"+url.PathEscape(id), nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "lead-list id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newLeadListVerificationStatsCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         "verification-stats",
		Annotations: readOnly,
		Short:       "Email-verification stats for a lead list (GET /lead-lists/{id}/verification-stats)",
		Long: "--id is required. Reports how the list's addresses came out of\n" +
			"verification — valid, risky, invalid, still unverified — which is the\n" +
			"check worth running before pushing a list into a campaign, since sending\n" +
			"to invalid addresses is what damages a sending account's reputation. It\n" +
			"verifies nothing itself; that is `verify create`.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.get(cmd, token, "/lead-lists/"+url.PathEscape(id)+"/verification-stats", nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "lead-list id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}
