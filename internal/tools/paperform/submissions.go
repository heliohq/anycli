package paperform

import (
	"net/url"

	"github.com/spf13/cobra"
)

// The four Longs for the submissions family live next to the shared builder
// because it is the builder that fixes which endpoint each pair reads —
// completed responses or abandoned ones — and that distinction is the whole
// content of the prose.
const (
	longSubmissionList = "The completed responses, and the payload most triage and summary work is\n" +
		"actually after. Answers arrive keyed by field key, so pair this with\n" +
		"`field list` to map them back to questions. Newest first unless `--sort\n" +
		"ASC`; narrow with the UTC `--after-date` / `--before-date` window or walk by\n" +
		"record with `--after-id` / `--before-id`. `--search` and `--search-fields`\n" +
		"belong to the shared flag set and search FORMS, not answers — there is no\n" +
		"full-text search over responses."

	longSubmissionGet = "`--id` is required; `--form` is optional and only scopes the lookup, since a\n" +
		"submission id resolves on its own. Reach for this when one response needs\n" +
		"reading in full — `submission list` already carries the answers, so paging\n" +
		"the list and then fetching each row is a wasted call per row."

	longPartialSubmissionList = "Partial submissions are responses a visitor STARTED and never submitted, so\n" +
		"this is a drop-off view, not a queue of work awaiting review — a partial\n" +
		"never becomes a submission and never appears in `submission list`. Most\n" +
		"triage wants that command instead. Paging, sorting and the date/id windows\n" +
		"behave exactly as they do there."

	longPartialSubmissionGet = "Reads one abandoned response in full, `--id` required and `--form` optional.\n" +
		"Expect gaps: the visitor stopped partway, so fields after that point are\n" +
		"absent rather than empty, and there is no completed twin of this record to\n" +
		"compare against."
)

// newSubmissionCmd builds the `submission` group: list a form's submissions and
// get one submission (with or without the parent form).
func (s *Service) newSubmissionCmd(key string) *cobra.Command {
	return s.newSubmissionGroup(key, "submission", "submissions",
		"List and read form submissions", longSubmissionList, longSubmissionGet)
}

// newPartialSubmissionCmd builds the `partial-submission` group: the same read
// surface as `submission`, over the partial-submissions endpoints.
func (s *Service) newPartialSubmissionCmd(key string) *cobra.Command {
	return s.newSubmissionGroup(key, "partial-submission", "partial-submissions",
		"List and read abandoned (partial) submissions", longPartialSubmissionList, longPartialSubmissionGet)
}

// newSubmissionGroup builds a list+get group for a submissions-family resource.
// segment is the URL path segment ("submissions" or "partial-submissions");
// word is the CLI command word. listLong/getLong carry the per-family prose.
func (s *Service) newSubmissionGroup(key, word, segment, short, listLong, getLong string) *cobra.Command {
	group := newGroupCmd(word, short)

	var lp listParams
	var listFormID string
	list := &cobra.Command{
		Use:         "list",
		Short:       "List a form's " + segment,
		Long:        listLong,
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
		Long:        getLong,
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
