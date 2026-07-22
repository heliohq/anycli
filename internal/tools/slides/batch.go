package slides

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

// batchUpdate posts a list of requests to presentations.batchUpdate. The whole
// batch is atomic: if any request fails validation, none are applied (API
// semantics, surfaced verbatim to the caller).
func (s *Service) batchUpdate(ctx context.Context, token, presentationID string, requests []any) ([]byte, error) {
	payload := map[string]any{"requests": requests}
	return s.call(ctx, token, http.MethodPost,
		"/presentations/"+presentationID+":batchUpdate", nil, payload)
}

// batchReply is the subset of a batchUpdate reply we surface: the object ids
// the API minted for create/duplicate requests.
type batchReply struct {
	CreateSlide     *objectIDReply `json:"createSlide"`
	CreateImage     *objectIDReply `json:"createImage"`
	DuplicateObject *objectIDReply `json:"duplicateObject"`
	ReplaceAllText  *struct {
		OccurrencesChanged int `json:"occurrencesChanged"`
	} `json:"replaceAllText"`
}

type objectIDReply struct {
	ObjectID string `json:"objectId"`
}

type batchUpdateResponse struct {
	PresentationID string       `json:"presentationId"`
	Replies        []batchReply `json:"replies"`
}

func (s *Service) newBatchUpdateCmd(token string) *cobra.Command {
	var requestsInline, requestsFile string
	cmd := &cobra.Command{
		Use:   "batch-update <presentation-id-or-url>",
		Short: "Escape hatch: pass raw batchUpdate requests through verbatim (all 44 request types)",
		Long: "Pass the full Slides batchUpdate request surface through verbatim. --requests accepts a " +
			"JSON array of Request objects, a single Request object, or a full {\"requests\":[...]} body. " +
			"The whole batch is atomic: if any request is invalid, none are applied.",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if (requestsInline == "") == (requestsFile == "") {
				return fmt.Errorf("slides: pass exactly one of --requests or --requests-file")
			}
			raw := []byte(requestsInline)
			if requestsFile != "" {
				b, err := os.ReadFile(requestsFile)
				if err != nil {
					return fmt.Errorf("slides: read requests file: %w", err)
				}
				raw = b
			}
			payload, err := normalizeBatchRequests(raw)
			if err != nil {
				return err
			}
			pid := extractPresentationID(args[0])
			body, err := s.call(cmd.Context(), token, http.MethodPost,
				"/presentations/"+pid+":batchUpdate", nil, payload)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			return s.renderBatchReplies(body)
		},
	}
	cmd.Flags().StringVar(&requestsInline, "requests", "", "raw JSON: a Request array, one Request object, or a full batchUpdate body")
	cmd.Flags().StringVar(&requestsFile, "requests-file", "", "path to a file holding the same raw JSON as --requests")
	return cmd
}

// normalizeBatchRequests coerces the three accepted --requests shapes into a
// batchUpdate body: a bare Request array, a single Request object, or an
// object that already carries a top-level "requests" key.
func normalizeBatchRequests(raw []byte) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("slides: --requests is empty")
	}
	if !json.Valid(trimmed) {
		return nil, fmt.Errorf("slides: --requests is not valid JSON")
	}
	switch trimmed[0] {
	case '[':
		return json.RawMessage(`{"requests":` + string(trimmed) + `}`), nil
	case '{':
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &probe); err != nil {
			return nil, fmt.Errorf("slides: decode --requests: %w", err)
		}
		if _, ok := probe["requests"]; ok {
			return json.RawMessage(trimmed), nil
		}
		return json.RawMessage(`{"requests":[` + string(trimmed) + `]}`), nil
	default:
		return nil, fmt.Errorf("slides: --requests must be a JSON array or object")
	}
}

// renderBatchReplies prints the minted object ids and replaced-text counts.
func (s *Service) renderBatchReplies(body []byte) error {
	var resp batchUpdateResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("slides: decode batchUpdate reply: %w", err)
	}
	fmt.Fprintf(s.stdout(), "batchUpdate applied (%d replies)\n", len(resp.Replies))
	for i, r := range resp.Replies {
		switch {
		case r.CreateSlide != nil:
			fmt.Fprintf(s.stdout(), "  [%d] createSlide -> %s\n", i, r.CreateSlide.ObjectID)
		case r.CreateImage != nil:
			fmt.Fprintf(s.stdout(), "  [%d] createImage -> %s\n", i, r.CreateImage.ObjectID)
		case r.DuplicateObject != nil:
			fmt.Fprintf(s.stdout(), "  [%d] duplicateObject -> %s\n", i, r.DuplicateObject.ObjectID)
		case r.ReplaceAllText != nil:
			fmt.Fprintf(s.stdout(), "  [%d] replaceAllText: %d occurrence(s) changed\n", i, r.ReplaceAllText.OccurrencesChanged)
		}
	}
	return nil
}
