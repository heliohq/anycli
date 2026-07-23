package buffer

import (
	"github.com/spf13/cobra"
)

const createIdeaMutation = `mutation($input: CreateIdeaInput!) {
  createIdea(input: $input) {
    __typename
    ... on Idea { id content { title text } }
    ... on MutationError { message }
  }
}`

func (s *Service) newIdeaCreateCmd(token string) *cobra.Command {
	var org, text, title string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create an idea in an organization",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if org == "" {
				return &usageError{msg: "--org is required"}
			}
			if text == "" {
				return &usageError{msg: "--text is required"}
			}
			content := map[string]any{"text": text}
			if title != "" {
				content["title"] = title
			}
			input := map[string]any{
				"organizationId": org,
				"content":        content,
			}
			data, err := s.gql(cmd.Context(), token, createIdeaMutation, map[string]any{"input": input})
			if err != nil {
				return err
			}
			payload, err := mutationSuccess(data, "createIdea", "Idea")
			if err != nil {
				return err
			}
			out := map[string]any{"id": payload["id"]}
			if content, ok := payload["content"].(map[string]any); ok {
				out["title"] = content["title"]
				out["text"] = content["text"]
			}
			return s.emitValue(out)
		},
	}
	cmd.Flags().StringVar(&org, "org", "", "organization (workspace) id (required)")
	cmd.Flags().StringVar(&text, "text", "", "idea body text (required)")
	cmd.Flags().StringVar(&title, "title", "", "idea title (optional)")
	return cmd
}
