package figma

import (
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newTeamsCommand(token string) *cobra.Command {
	teams := &cobra.Command{Use: "teams", Short: "Discover Figma team content"}
	teams.AddCommand(s.newTeamProjectsCommand(token))
	return teams
}

func (s *Service) newTeamProjectsCommand(token string) *cobra.Command {
	var teamID string
	cmd := &cobra.Command{
		Use:   "projects",
		Short: "List projects visible in a Figma team",
		Args:  cobra.NoArgs,
		Annotations: map[string]string{
			operationIDAnnotation: "getTeamProjects",
			sideEffectAnnotation:  "false", // GET team projects
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.callCatalogOperationAndEmit(cmd.Context(), token, "getTeamProjects", []string{"team_id=" + teamID}, nil)
		},
	}
	cmd.Flags().StringVar(&teamID, "team-id", "", "Figma team ID from a team URL")
	_ = cmd.MarkFlagRequired("team-id")
	return cmd
}

func (s *Service) newProjectsCommand(token string) *cobra.Command {
	projects := &cobra.Command{Use: "projects", Short: "Read Figma projects"}
	projects.AddCommand(
		s.newProjectMetaCommand(token),
		s.newProjectFilesCommand(token),
	)
	return projects
}

func (s *Service) newProjectMetaCommand(token string) *cobra.Command {
	var projectID string
	cmd := &cobra.Command{
		Use:   "meta",
		Short: "Get Figma project metadata",
		Args:  cobra.NoArgs,
		Annotations: map[string]string{
			operationIDAnnotation: "getProjectMeta",
			sideEffectAnnotation:  "false", // GET project metadata
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.callCatalogOperationAndEmit(cmd.Context(), token, "getProjectMeta", []string{"project_id=" + projectID}, nil)
		},
	}
	cmd.Flags().StringVar(&projectID, "project-id", "", "Figma project ID")
	_ = cmd.MarkFlagRequired("project-id")
	return cmd
}

func (s *Service) newProjectFilesCommand(token string) *cobra.Command {
	var projectID string
	var branchData bool
	cmd := &cobra.Command{
		Use:   "files",
		Short: "List files in a Figma project",
		Args:  cobra.NoArgs,
		Annotations: map[string]string{
			operationIDAnnotation: "getProjectFiles",
			sideEffectAnnotation:  "false", // GET project files
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			query := url.Values{}
			if branchData {
				query.Set("branch_data", "true")
			}
			params := appendOperationQuery([]string{"project_id=" + projectID}, query)
			return s.callCatalogOperationAndEmit(cmd.Context(), token, "getProjectFiles", params, nil)
		},
	}
	cmd.Flags().StringVar(&projectID, "project-id", "", "Figma project ID")
	cmd.Flags().BoolVar(&branchData, "branch-data", false, "include branch metadata")
	_ = cmd.MarkFlagRequired("project-id")
	return cmd
}
