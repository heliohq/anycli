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
	isPublished          bool
	isAcceptingResponses bool
	doneMsg              string
}

var (
	publishOp = publishVerb{
		use:                  "publish <form-id>",
		short:                "Publish the form and start accepting responses (setPublishSettings)",
		isPublished:          true,
		isAcceptingResponses: true,
		doneMsg:              "published form %s (now accepting responses)\n",
	}
	unpublishOp = publishVerb{
		use:                  "unpublish <form-id>",
		short:                "Take the form fully offline — responders can no longer see it",
		isPublished:          false,
		isAcceptingResponses: false,
		doneMsg:              "unpublished form %s\n",
	}
	closeOp = publishVerb{
		use:                  "close <form-id>",
		short:                "Stop accepting responses while keeping the form published",
		isPublished:          true,
		isAcceptingResponses: false,
		doneMsg:              "closed form %s (still published, no longer accepting responses)\n",
	}
	reopenOp = publishVerb{
		use:                  "reopen <form-id>",
		short:                "Resume accepting responses on a published form",
		isPublished:          true,
		isAcceptingResponses: true,
		doneMsg:              "reopened form %s (accepting responses)\n",
	}
)

func (s *Service) newPublishCmd(token string, v publishVerb) *cobra.Command {
	return &cobra.Command{
		Use:   v.use,
		Short: v.short,
		Args:  cobra.ExactArgs(1),
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
