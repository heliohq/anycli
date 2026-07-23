package klaviyo

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newCollectionListCmd builds a generic collection GET with the shared JSON:API
// query flags. resourceType feeds the --fields sparse-fieldset param.
func (s *Service) newCollectionListCmd(token, use, short, path, resourceType string) *cobra.Command {
	f := &listFlags{}
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q, err := f.query(resourceType)
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, path, q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	registerListFlags(cmd, f)
	return cmd
}

// newResourceGetCmd builds a generic single-resource GET (pathPrefix + id) with
// the shared query flags for sparse fieldsets / includes.
func (s *Service) newResourceGetCmd(token, use, short, pathPrefix, resourceType string) *cobra.Command {
	f := &listFlags{}
	cmd := &cobra.Command{
		Use:         use + " <id>",
		Short:       short,
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			q, err := f.query(resourceType)
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, pathPrefix+url.PathEscape(args[0]), q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	registerListFlags(cmd, f)
	return cmd
}
