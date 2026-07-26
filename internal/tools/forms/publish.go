package forms

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// publishVerb describes one thin synthetic verb over setPublishSettings. The
// four natural verbs replace the raw --published/--accepting boolean pair and
// make the destructive gradient legible: publish sits alone at the top
// (outward-facing), the other three are convergent (reversible) directions.
type publishVerb struct {
	use                  string
	short                string
	long                 string
	isPublished          bool
	isAcceptingResponses bool
	doneMsg              string
}

var (
	publishOp = publishVerb{
		use:   "publish <form-id>",
		short: "Publish the form and start accepting responses (setPublishSettings)",
		long: "Makes the form reachable at its `responderUri` and starts collecting\n" +
			"answers from real people — the point after which the questions are no\n" +
			"longer a draft. Check the structure with `get` first, since editing a live\n" +
			"form changes what later respondents see. Publishing alone does not decide\n" +
			"WHO can answer; that is `responders add`. Reversible in two directions:\n" +
			"`close` stops answers but keeps the page up, `unpublish` removes it\n" +
			"entirely.",
		isPublished:          true,
		isAcceptingResponses: true,
		doneMsg:              "published form %s (now accepting responses)\n",
	}
	unpublishOp = publishVerb{
		use:   "unpublish <form-id>",
		short: "Take the form fully offline — responders can no longer see it",
		long: "The responder URL stops resolving for everyone, so someone holding the link\n" +
			"sees nothing rather than a closed form — `close` is the gentler option when\n" +
			"the form should stay visible. Responses already collected are untouched and\n" +
			"still readable through `responses list`. `publish` brings it back at the\n" +
			"same URL.",
		isPublished:          false,
		isAcceptingResponses: false,
		doneMsg:              "unpublished form %s\n",
	}
	closeOp = publishVerb{
		use:   "close <form-id>",
		short: "Stop accepting responses while keeping the form published",
		long: "The form stays published and its link keeps working; visitors are told it\n" +
			"is no longer accepting responses instead of hitting a dead URL, which is\n" +
			"the whole difference from `unpublish`. Sharing is untouched, so `reopen`\n" +
			"resumes collection for exactly the same audience.",
		isPublished:          true,
		isAcceptingResponses: false,
		doneMsg:              "closed form %s (still published, no longer accepting responses)\n",
	}
	reopenOp = publishVerb{
		use:   "reopen <form-id>",
		short: "Resume accepting responses on a published form",
		long: "Undoes `close`: the form starts taking answers again from everyone it was\n" +
			"already shared with, with no further sharing step. Outward-facing in the\n" +
			"same way `publish` is — real people can submit the moment it returns. It\n" +
			"sends the same publish state as `publish`, so it is also a way to confirm a\n" +
			"form is live.",
		isPublished:          true,
		isAcceptingResponses: true,
		doneMsg:              "reopened form %s (accepting responses)\n",
	}
)

func (s *Service) newPublishCmd(token string, v publishVerb) *cobra.Command {
	return &cobra.Command{
		Use:   v.use,
		Short: v.short,
		Long:  v.long,
		Args:  cobra.ExactArgs(1),
		// POST /forms/{id}:setPublishSettings — mutating provider call, all
		// four verbs (design 318).
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			formID, err := extractFormID(args[0])
			if err != nil {
				return err
			}
			payload := map[string]any{
				"publishSettings": map[string]any{
					"publishState": map[string]any{
						"isPublished":          v.isPublished,
						"isAcceptingResponses": v.isAcceptingResponses,
					},
				},
				"updateMask": "publishState",
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/forms/"+url.PathEscape(formID)+":setPublishSettings", nil, payload)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			fmt.Fprintf(s.stdout(), v.doneMsg, formID)
			return nil
		},
	}
}
