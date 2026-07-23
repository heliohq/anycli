package dataforseo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
)

// newMetaCmd is the `meta` resource group: the location and language reference
// lists. Identifiers are the #1 friction for agents, so these fetch the server
// lists and filter them client-side by an optional --search substring.
func (s *Service) newMetaCmd(credential string) *cobra.Command {
	meta := newGroupCmd("meta", "Location and language reference lists")
	meta.AddCommand(
		s.newMetaLocationsCmd(credential),
		s.newMetaLanguagesCmd(credential),
	)
	return meta
}

// newMetaLocationsCmd lists Google SERP locations, optionally filtered.
func (s *Service) newMetaLocationsCmd(credential string) *cobra.Command {
	var search string
	cmd := &cobra.Command{
		Use:         "locations",
		Short:       "List Google SERP locations (name + location_code)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.doFiltered(cmd.Context(), credential, "/serp/google/locations", search, "location_name")
		},
	}
	cmd.Flags().StringVar(&search, "search", "", "case-insensitive substring filter on the location name")
	return cmd
}

// newMetaLanguagesCmd lists Google SERP languages, optionally filtered.
func (s *Service) newMetaLanguagesCmd(credential string) *cobra.Command {
	var search string
	cmd := &cobra.Command{
		Use:         "languages",
		Short:       "List Google SERP languages (name + language_code)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.doFiltered(cmd.Context(), credential, "/serp/google/languages", search, "language_name")
		},
	}
	cmd.Flags().StringVar(&search, "search", "", "case-insensitive substring filter on the language name")
	return cmd
}

// doFiltered GETs a reference list, filters result entries whose nameField
// contains the (case-insensitive) search substring, and emits {cost, result}.
func (s *Service) doFiltered(ctx context.Context, credential, path, search, nameField string) error {
	res, err := s.callAPI(ctx, credential, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	if search == "" || len(res.Result) == 0 {
		return s.emit(res)
	}
	var entries []map[string]any
	if uErr := json.Unmarshal(res.Result, &entries); uErr != nil {
		// Not the expected array shape — emit unfiltered rather than fail.
		return s.emit(res)
	}
	needle := strings.ToLower(search)
	filtered := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		if name, ok := e[nameField].(string); ok && strings.Contains(strings.ToLower(name), needle) {
			filtered = append(filtered, e)
		}
	}
	b, mErr := json.Marshal(filtered)
	if mErr != nil {
		return &apiError{msg: fmt.Sprintf("dataforseo: encode filtered result: %v", mErr), err: mErr}
	}
	res.Result = b
	return s.emit(res)
}
