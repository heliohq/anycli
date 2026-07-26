package drive

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// validRoles are the roles this tool exposes for sharing. ownership transfer
// (role=owner) is intentionally omitted — it is a distinct, high-risk action
// outside v1.
var validRoles = map[string]bool{"reader": true, "commenter": true, "writer": true}

// permission is the decoded shape of a Drive permission.
type permission struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	Role         string `json:"role"`
	EmailAddress string `json:"emailAddress"`
	Domain       string `json:"domain"`
	DisplayName  string `json:"displayName"`
}

func (s *Service) newPermissionsListCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "list <file-id>",
		Short: "List who a file is shared with (permissions.list)",
		Long: "Every grant on the file, each carrying the permission `id` that\n" +
			"`permissions update` and `permissions delete` require — those take that\n" +
			"id, never an email address. A permission of type `anyone` means the file\n" +
			"is readable by whoever holds the link, which is the entry to look for when\n" +
			"auditing exposure.",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			q := driveParams()
			q.Set("fields", "permissions(id,type,role,emailAddress,domain,displayName)")
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/files/"+url.PathEscape(args[0])+"/permissions", q, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				Permissions []permission `json:"permissions"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("drive: decode permissions: %w", err)
			}
			if len(resp.Permissions) == 0 {
				fmt.Fprintln(s.stdout(), "no permissions")
				return nil
			}
			for _, p := range resp.Permissions {
				fmt.Fprintf(s.stdout(), "%s\t%s\t%s\t%s\n", p.ID, p.Role, p.Type, permTarget(p.EmailAddress, p.Domain))
			}
			return nil
		},
	}
}

// newFilesShareCmd is the synthetic `files share` verb over permissions.create.
// It is the tool's outward-exposure surface — the skill contract requires the
// assistant to confirm what/who/role with the user before running it,
// especially --anyone.
func (s *Service) newFilesShareCmd(token string) *cobra.Command {
	var with []string
	var anyone, noNotify bool
	var role, message string
	cmd := &cobra.Command{
		Use:   "share <file-id> --with a@b[,c@d] --role reader|commenter|writer",
		Short: "Share a file (permissions.create). Outward-facing — confirm with the user first; --anyone makes it link-visible.",
		Long: "The point at which the file leaves the connected account's private domain.\n" +
			"--with takes one or more addresses; --anyone makes it readable by ANYONE\n" +
			"holding the URL, which is the highest-exposure form because a link travels\n" +
			"further than the intent behind it. The two are mutually exclusive. --role\n" +
			"defaults to reader and accepts commenter or writer.\n" +
			"\n" +
			"Each recipient is a separate API call, so this is NOT atomic — a failure\n" +
			"part-way leaves the grants already made in place. Addressed recipients get\n" +
			"a notification email carrying --message unless --no-notify is passed; an\n" +
			"`anyone` grant never notifies anyone.",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if !validRoles[role] {
				return fmt.Errorf("drive: --role must be reader, commenter, or writer, got %q", role)
			}
			if anyone && len(with) > 0 {
				return fmt.Errorf("drive: --anyone and --with are mutually exclusive")
			}
			if !anyone && len(with) == 0 {
				return fmt.Errorf("drive: share needs --with <email>[,<email>] or --anyone")
			}

			targets := []map[string]any{}
			if anyone {
				targets = append(targets, map[string]any{"type": "anyone", "role": role})
			}
			for _, addr := range splitEmails(with) {
				targets = append(targets, map[string]any{"type": "user", "role": role, "emailAddress": addr})
			}

			q := driveParams()
			q.Set("fields", "id,type,role,emailAddress,domain")
			// permissions.create notifies user/group grants by default; keep
			// the notification unless the caller opts out. anyone-type grants
			// never notify.
			if noNotify {
				q.Set("sendNotificationEmail", "false")
			}
			if message != "" {
				q.Set("emailMessage", message)
			}

			created := make([]permission, 0, len(targets))
			for _, body := range targets {
				respBody, err := s.call(cmd.Context(), token, http.MethodPost, "/files/"+url.PathEscape(args[0])+"/permissions", q, body)
				if err != nil {
					return err
				}
				var p permission
				if err := json.Unmarshal(respBody, &p); err != nil {
					return fmt.Errorf("drive: decode permission: %w", err)
				}
				created = append(created, p)
			}
			if jsonOut(cmd) {
				return s.emitJSON(map[string]any{"fileId": args[0], "permissions": created})
			}
			for _, p := range created {
				fmt.Fprintf(s.stdout(), "shared %s as %s with %s\n", args[0], p.Role, permTarget(p.EmailAddress, p.Domain))
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&with, "with", nil, "recipient email(s), comma-separated or repeated")
	cmd.Flags().BoolVar(&anyone, "anyone", false, "make link-visible to anyone with the link (highest exposure — confirm with the user)")
	cmd.Flags().StringVar(&role, "role", "reader", "reader, commenter, or writer")
	cmd.Flags().StringVar(&message, "message", "", "custom message in the notification email")
	cmd.Flags().BoolVar(&noNotify, "no-notify", false, "do not send a notification email to user/group recipients")
	return cmd
}

func (s *Service) newPermissionsUpdateCmd(token string) *cobra.Command {
	var role string
	cmd := &cobra.Command{
		Use:   "update <file-id> <permission-id> --role reader|commenter|writer",
		Short: "Change a permission's role. Escalation (e.g. reader→writer) widens exposure — confirm with the user first.",
		Long: "Takes the file id and the PERMISSION id from `permissions list`, not an\n" +
			"email address. Raising a role — reader to writer — lets that grantee change\n" +
			"or delete the file's contents; lowering it narrows exposure and takes\n" +
			"effect immediately. Neither direction sends any notification, so someone\n" +
			"who loses write access simply finds it gone.",
		Args:        cobra.ExactArgs(2),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if !validRoles[role] {
				return fmt.Errorf("drive: --role must be reader, commenter, or writer, got %q", role)
			}
			q := driveParams()
			q.Set("fields", "id,type,role,emailAddress,domain")
			body, err := s.call(cmd.Context(), token, http.MethodPatch,
				"/files/"+url.PathEscape(args[0])+"/permissions/"+url.PathEscape(args[1]), q, map[string]any{"role": role})
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var p permission
			if err := json.Unmarshal(body, &p); err != nil {
				return fmt.Errorf("drive: decode permission: %w", err)
			}
			fmt.Fprintf(s.stdout(), "updated permission %s to %s\n", p.ID, p.Role)
			return nil
		},
	}
	cmd.Flags().StringVar(&role, "role", "", "new role: reader, commenter, or writer")
	_ = cmd.MarkFlagRequired("role")
	return cmd
}

func (s *Service) newPermissionsDeleteCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <file-id> <permission-id>",
		Short: "Revoke a permission (permissions.delete). Convergent — safe to run without confirmation.",
		Long: "Takes the file id and the permission id from `permissions list`. Revocation\n" +
			"is immediate and silent — nothing emails the person who lost access.\n" +
			"Deleting the `anyone` permission is what turns a link-shared file private\n" +
			"again, and is the narrowing counterpart to `files share --anyone`.",
		Args:        cobra.ExactArgs(2),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			q := driveParams()
			if _, err := s.call(cmd.Context(), token, http.MethodDelete,
				"/files/"+url.PathEscape(args[0])+"/permissions/"+url.PathEscape(args[1]), q, nil); err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitJSON(map[string]any{"fileId": args[0], "permissionId": args[1], "status": "deleted"})
			}
			fmt.Fprintf(s.stdout(), "deleted permission %s\n", args[1])
			return nil
		},
	}
}

// splitEmails splits comma-joined --with values and drops empties.
func splitEmails(with []string) []string {
	out := []string{}
	for _, v := range with {
		for _, part := range strings.Split(v, ",") {
			if p := strings.TrimSpace(part); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}
