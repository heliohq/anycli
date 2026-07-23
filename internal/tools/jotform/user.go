package jotform

import "github.com/spf13/cobra"

func (s *Service) newUserCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:         "user",
		Short:       "Get the authenticated account (GET /user)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.get(cmd.Context(), key, "/user", nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newUsageCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:         "usage",
		Short:       "Get API/usage counters for the account (GET /user/usage)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.get(cmd.Context(), key, "/user/usage", nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}
