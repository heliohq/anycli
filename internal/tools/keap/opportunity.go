package keap

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newOpportunityCmd(token string) *cobra.Command {
	cmd := newGroupCmd("opportunity", "Opportunities (list, get, create, update, stages)")
	cmd.AddCommand(
		s.newOpportunityListCmd(token),
		s.newOpportunityGetCmd(token),
		s.newOpportunityCreateCmd(token),
		s.newOpportunityUpdateCmd(token),
		s.newOpportunityStagesCmd(token),
	)
	return cmd
}

func (s *Service) newOpportunityListCmd(token string) *cobra.Command {
	var lf *listFlags
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List opportunities (GET /v2/opportunities)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/opportunities", lf.values(), nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	lf = registerListFlags(cmd)
	return cmd
}

func (s *Service) newOpportunityGetCmd(token string) *cobra.Command {
	var fields string
	cmd := &cobra.Command{
		Use:         "get <opportunity-id>",
		Short:       "Get an opportunity (GET /v2/opportunities/{id})",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/opportunities/"+url.PathEscape(args[0]), fieldsQuery(fields), nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&fields, "fields", "", "comma-separated fields to include")
	return cmd
}

// opportunityBodyFlags holds the convenience field flags shared by create/update.
type opportunityBodyFlags struct {
	title, contactID, stageID, userID string
	jsonBody                          string
}

func registerOpportunityBodyFlags(cmd *cobra.Command) *opportunityBodyFlags {
	f := &opportunityBodyFlags{}
	cmd.Flags().StringVar(&f.title, "title", "", "opportunity title")
	cmd.Flags().StringVar(&f.contactID, "contact-id", "", "associated contact id")
	cmd.Flags().StringVar(&f.stageID, "stage-id", "", "pipeline stage id")
	cmd.Flags().StringVar(&f.userID, "user-id", "", "owning user id")
	cmd.Flags().StringVar(&f.jsonBody, "json-body", "", "raw JSON body merged over the flag-built payload")
	return f
}

func (f *opportunityBodyFlags) build() (map[string]any, error) {
	body := map[string]any{}
	if f.title != "" {
		body["opportunity_title"] = f.title
	}
	if f.contactID != "" {
		body["contact_id"] = f.contactID
	}
	if f.stageID != "" {
		body["stage_id"] = f.stageID
	}
	if f.userID != "" {
		body["user_id"] = f.userID
	}
	if err := applyJSONBody(body, f.jsonBody); err != nil {
		return nil, err
	}
	return body, nil
}

func (s *Service) newOpportunityCreateCmd(token string) *cobra.Command {
	var f *opportunityBodyFlags
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create an opportunity (POST /v2/opportunities)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := f.build()
			if err != nil {
				return err
			}
			if _, ok := body["opportunity_title"]; !ok {
				return &usageError{msg: "opportunity create requires --title (or opportunity_title in --json-body)"}
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/v2/opportunities", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	f = registerOpportunityBodyFlags(cmd)
	return cmd
}

func (s *Service) newOpportunityUpdateCmd(token string) *cobra.Command {
	var f *opportunityBodyFlags
	cmd := &cobra.Command{
		Use:         "update <opportunity-id>",
		Short:       "Update an opportunity (PATCH /v2/opportunities/{id})",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := f.build()
			if err != nil {
				return err
			}
			if err := requireBody(body); err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPatch, "/v2/opportunities/"+url.PathEscape(args[0]), nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	f = registerOpportunityBodyFlags(cmd)
	return cmd
}

func (s *Service) newOpportunityStagesCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "stages",
		Short:       "List opportunity stages (GET /v2/opportunities/stages)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/opportunities/stages", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}
