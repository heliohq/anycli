package resend

import (
	"net/http"

	"github.com/spf13/cobra"
)

func (s *Service) newDomainCmd(key string) *cobra.Command {
	cmd := newGroupCmd("domain", "Manage sending domains (list, get, create, verify, update, delete)")
	cmd.AddCommand(
		s.newDomainListCmd(key),
		s.newDomainGetCmd(key),
		s.newDomainCreateCmd(key),
		s.newDomainVerifyCmd(key),
		s.newDomainUpdateCmd(key),
		s.newDomainDeleteCmd(key),
	)
	return cmd
}

func (s *Service) newDomainListCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List sending domains (GET /domains)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/domains", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newDomainGetCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Retrieve a domain (GET /domains/{id})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/domains/"+args[0], nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newDomainCreateCmd(key string) *cobra.Command {
	var name, region string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Add a sending domain (POST /domains)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{"name": name}
			if region != "" {
				body["region"] = region
			}
			resp, err := s.call(cmd.Context(), key, http.MethodPost, "/domains", body, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "domain name, e.g. example.com")
	cmd.Flags().StringVar(&region, "region", "", "region: us-east-1 | eu-west-1 | sa-east-1 | ap-northeast-1")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func (s *Service) newDomainVerifyCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:   "verify <id>",
		Short: "Trigger domain DNS verification (POST /domains/{id}/verify)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), key, http.MethodPost, "/domains/"+args[0]+"/verify", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newDomainUpdateCmd(key string) *cobra.Command {
	var openTracking, clickTracking bool
	var tls string
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a domain's tracking settings (PATCH /domains/{id})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{}
			if cmd.Flags().Changed("open-tracking") {
				body["open_tracking"] = openTracking
			}
			if cmd.Flags().Changed("click-tracking") {
				body["click_tracking"] = clickTracking
			}
			if tls != "" {
				body["tls"] = tls
			}
			resp, err := s.call(cmd.Context(), key, http.MethodPatch, "/domains/"+args[0], body, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().BoolVar(&openTracking, "open-tracking", false, "enable open tracking")
	cmd.Flags().BoolVar(&clickTracking, "click-tracking", false, "enable click tracking")
	cmd.Flags().StringVar(&tls, "tls", "", "TLS policy: opportunistic | enforced")
	return cmd
}

func (s *Service) newDomainDeleteCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a domain (DELETE /domains/{id})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), key, http.MethodDelete, "/domains/"+args[0], nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}
