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
		Long: "The authoritative answer to what any `--from` is allowed to be: each\n" +
			"domain carries its verification status and its region. A domain still\n" +
			"pending cannot send. Reading this first is cheaper than learning the\n" +
			"same thing from a rejected send.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "Takes the domain id from `domain list`, not the domain name. The\n" +
			"response carries the DNS records Resend expects — SPF, DKIM and the\n" +
			"return-path entry — each with its own status, which is how to tell\n" +
			"someone exactly which record is still missing at their registrar rather\n" +
			"than \"verification failed\".",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
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
		Long: "Registers the domain and returns the DNS records that must then be added\n" +
			"at the registrar. It does NOT make the domain usable: sending stays\n" +
			"blocked until those records propagate and `domain verify` passes.\n" +
			"--region fixes where mail leaves from (us-east-1, eu-west-1, sa-east-1,\n" +
			"ap-northeast-1) and is chosen once, at creation.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
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
		Long: "Asks Resend to re-check the domain's DNS now. It creates and changes\n" +
			"nothing — the records live in the user's registrar — so running it\n" +
			"before they exist and have propagated simply leaves the domain\n" +
			"unverified. Propagation takes minutes to hours, and re-running this is\n" +
			"the only way to find out that it finished.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
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
		Long: "Changes delivery-time behaviour only. Click tracking rewrites every link\n" +
			"in outgoing mail through a Resend redirect, which some corporate\n" +
			"security scanners treat as suspicious. --tls enforced makes a send FAIL\n" +
			"rather than fall back to an unencrypted connection, where opportunistic\n" +
			"downgrades silently. The two boolean flags are sent only when actually\n" +
			"passed, so an untouched setting is left alone.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
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
		Long: "After this, no address on the domain can send anything. The DNS records\n" +
			"at the registrar are untouched and have to be cleaned up separately, and\n" +
			"re-adding the domain later starts verification over from the beginning.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), key, http.MethodDelete, "/domains/"+args[0], nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}
