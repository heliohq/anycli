package posthog

import (
	"github.com/spf13/cobra"
)

// longAnnotationList lives here because the leaf is built by the shared
// project-list constructor.
const longAnnotationList = "Annotations are project-scoped markers drawn on the analytics timeline,\n" +
	"ordered by the instant they mark rather than by creation time. This endpoint\n" +
	"takes no `--search`, so narrowing to one deploy means paging with `--limit`\n" +
	"/ `--offset` and filtering the `content` field locally."

// newAnnotationCmd groups annotation read and create access — the surface an
// agent uses to mark deploys and launches on the analytics timeline.
func (s *Service) newAnnotationCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "annotation", Short: "Annotations (list, create)"}
	cmd.AddCommand(
		s.newProjectListCmd(token, "list", "List annotations (GET /api/projects/<id>/annotations/)", longAnnotationList, "/annotations/", false),
		s.newAnnotationCreateCmd(token),
	)
	return cmd
}

func (s *Service) newAnnotationCreateCmd(token string) *cobra.Command {
	var project, content, dateMarker, scope string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an annotation (POST /api/projects/<id>/annotations/)",
		Long: "--content is the marker text and is required. --date-marker is the ISO-8601\n" +
			"instant the marker points at; omitted, PostHog stamps it now, so\n" +
			"backfilling a past deploy means passing it explicitly. --scope is project\n" +
			"or organization — an organization-scoped annotation appears on every\n" +
			"project's charts, not just this one.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireProject(project); err != nil {
				return err
			}
			if err := requireFlag("content", content); err != nil {
				return err
			}
			payload := map[string]any{"content": content}
			if dateMarker != "" {
				payload["date_marker"] = dateMarker
			}
			if scope != "" {
				payload["scope"] = scope
			}
			resp, err := s.call(cmd.Context(), token, "POST", projectPath(project, "/annotations/"), nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project id (required)")
	cmd.Flags().StringVar(&content, "content", "", "annotation text (required)")
	cmd.Flags().StringVar(&dateMarker, "date-marker", "", "ISO-8601 timestamp the annotation marks (optional)")
	cmd.Flags().StringVar(&scope, "scope", "", "annotation scope: project|organization (optional)")
	return cmd
}
