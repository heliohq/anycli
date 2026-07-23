package paperform

import (
	"net/url"

	"github.com/spf13/cobra"
)

// newSubmissionCmd builds the `submission` group: list a form's submissions and
// get one submission (with or without the parent form).
func (s *Service) newSubmissionCmd(key string) *cobra.Command {
	return s.newSubmissionGroup(key, "submission", "submissions",
		"List and read form submissions")
}

// newPartialSubmissionCmd builds the `partial-submission` group: the same read
// surface as `submission`, over the partial-submissions endpoints.
func (s *Service) newPartialSubmissionCmd(key string) *cobra.Command {
	return s.newSubmissionGroup(key, "partial-submission", "partial-submissions",
		"List and read abandoned (partial) submissions")
}

// newSubmissionGroup builds a list+get group for a submissions-family resource.
// segment is the URL path segment ("submissions" or "partial-submissions");
// word is the CLI command word.
func (s *Service) newSubmissionGroup(key, word, segment, short string) *cobra.Command {
	group := newGroupCmd(word, short)

	var lp listParams
	var listFormID string
	list := &cobra.Command{
		Use:         "list",
		Short:       "List a form's " + segment,
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if listFormID == "" {
				return &usageError{msg: word + " list: --form is required"}
			}
			return s.runGet(cmd, key, "/forms/"+url.PathEscape(listFormID)+"/"+segment, lp.query())
		},
	}
	list.Flags().StringVar(&listFormID, "form", "", "form slug or ID (required)")
	registerListFlags(list, &lp)

	var getID, getFormID string
	get := &cobra.Command{
		Use:         "get",
		Short:       "Get a single " + word + " by ID",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if getID == "" {
				return &usageError{msg: word + " get: --id is required"}
			}
			// The form-scoped path and the top-level path both resolve one
			// record; --form is optional and only narrows the lookup.
			path := "/" + segment + "/" + url.PathEscape(getID)
			if getFormID != "" {
				path = "/forms/" + url.PathEscape(getFormID) + "/" + segment + "/" + url.PathEscape(getID)
			}
			return s.runGet(cmd, key, path, nil)
		},
	}
	get.Flags().StringVar(&getID, "id", "", "submission ID (required)")
	get.Flags().StringVar(&getFormID, "form", "", "form slug or ID (optional; narrows the lookup)")

	group.AddCommand(list, get)
	return group
}
