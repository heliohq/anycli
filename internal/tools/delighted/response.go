package delighted

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newResponseCmd wires the survey-response resource: list, get, create, update
// over /survey_responses(.json).
func (s *Service) newResponseCmd(key string) *cobra.Command {
	cmd := &cobra.Command{Use: "response", Short: "Survey responses (verbatim feedback + scores)"}
	cmd.AddCommand(
		s.newResponseListCmd(key),
		s.newResponseGetCmd(key),
		s.newResponseCreateCmd(key),
		s.newResponseUpdateCmd(key),
	)
	return cmd
}

func (s *Service) newResponseListCmd(key string) *cobra.Command {
	var since, until, updatedSince, order, expand string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List survey responses (GET /survey_responses.json)",
		Args:  cobra.NoArgs,
	}
	perPage, page := registerPaging(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		applyPaging(q, *perPage, *page)
		setIfNonEmpty(q, "since", since)
		setIfNonEmpty(q, "until", until)
		setIfNonEmpty(q, "updated_since", updatedSince)
		setIfNonEmpty(q, "order", order)
		if expand != "" {
			q.Set("expand[]", expand)
		}
		resp, err := s.call(cmd.Context(), key, http.MethodGet, "/survey_responses.json", q, nil)
		if err != nil {
			return err
		}
		return s.emit(resp)
	}
	cmd.Flags().StringVar(&since, "since", "", "only responses created at/after this Unix timestamp")
	cmd.Flags().StringVar(&until, "until", "", "only responses created at/before this Unix timestamp")
	cmd.Flags().StringVar(&updatedSince, "updated-since", "", "only responses updated at/after this Unix timestamp")
	cmd.Flags().StringVar(&order, "order", "", "sort order: asc or desc")
	cmd.Flags().StringVar(&expand, "expand", "", "expand a nested object, e.g. person")
	return cmd
}

func (s *Service) newResponseGetCmd(key string) *cobra.Command {
	var id, expand string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Retrieve one survey response (GET /survey_responses/{id}.json)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if expand != "" {
				q.Set("expand[]", expand)
			}
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/survey_responses/"+url.PathEscape(id)+".json", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "survey response id")
	cmd.Flags().StringVar(&expand, "expand", "", "expand a nested object, e.g. person")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newResponseCreateCmd(key string) *cobra.Command {
	var person, score, comment, properties string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a survey response (POST /survey_responses.json)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload := map[string]any{"person": person, "score": score}
			if comment != "" {
				payload["comment"] = comment
			}
			if properties != "" {
				v, err := decodeJSONFlag("properties-json", properties)
				if err != nil {
					return err
				}
				payload["person_properties"] = v
			}
			resp, err := s.call(cmd.Context(), key, http.MethodPost, "/survey_responses.json", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&person, "person", "", "person id the response belongs to")
	cmd.Flags().StringVar(&score, "score", "", "survey score (e.g. 0-10 for NPS)")
	cmd.Flags().StringVar(&comment, "comment", "", "verbatim comment")
	cmd.Flags().StringVar(&properties, "properties-json", "", "person_properties as a raw JSON object")
	_ = cmd.MarkFlagRequired("person")
	_ = cmd.MarkFlagRequired("score")
	return cmd
}

func (s *Service) newResponseUpdateCmd(key string) *cobra.Command {
	var id, notes, properties string
	var tags []string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a survey response's tags/notes (PUT /survey_responses/{id}.json)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload := map[string]any{}
			if cmd.Flags().Changed("tags") {
				payload["tags"] = tags
			}
			if notes != "" {
				payload["notes"] = notes
			}
			if properties != "" {
				v, err := decodeJSONFlag("properties-json", properties)
				if err != nil {
					return err
				}
				payload["person_properties"] = v
			}
			resp, err := s.call(cmd.Context(), key, http.MethodPut, "/survey_responses/"+url.PathEscape(id)+".json", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "survey response id")
	cmd.Flags().StringSliceVar(&tags, "tags", nil, "replace the response's tags (comma-separated)")
	cmd.Flags().StringVar(&notes, "notes", "", "internal note text")
	cmd.Flags().StringVar(&properties, "properties-json", "", "person_properties as a raw JSON object")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}
