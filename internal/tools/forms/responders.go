package forms

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// permission is a Drive v3 permission row restricted to the published view.
type permission struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	Role         string `json:"role"`
	View         string `json:"view"`
	EmailAddress string `json:"emailAddress"`
	Domain       string `json:"domain"`
}

// listResponders fetches the published-view permissions on a form. Editors of
// the form (permissions with no view) are excluded — only responder rows.
func (s *Service) listResponders(ctx context.Context, token, formID string) ([]permission, error) {
	q := url.Values{
		"includePermissionsForView": {"published"},
		"fields":                    {"permissions(id,type,role,view,emailAddress,domain)"},
	}
	body, err := s.callDrive(ctx, token, http.MethodGet, "/files/"+url.PathEscape(formID)+"/permissions", q, nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Permissions []permission `json:"permissions"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("forms: decode permissions: %w", err)
	}
	out := make([]permission, 0, len(resp.Permissions))
	for _, p := range resp.Permissions {
		if p.View == "published" {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *Service) newRespondersListCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "list <form-id>",
		Short: "List who can answer the form (Drive permissions on the published view)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			formID, err := extractFormID(args[0])
			if err != nil {
				return err
			}
			perms, err := s.listResponders(cmd.Context(), token, formID)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitJSON(map[string]any{"responders": perms})
			}
			if len(perms) == 0 {
				fmt.Fprintln(s.stdout(), "no responders (not shared)")
				return nil
			}
			for _, p := range perms {
				who := p.EmailAddress
				if p.Type == "anyone" {
					who = "anyone-with-link"
				} else if p.Domain != "" {
					who = "domain:" + p.Domain
				}
				fmt.Fprintf(s.stdout(), "%s\ttype=%s\trole=%s\tid=%s\n", who, p.Type, p.Role, p.ID)
			}
			return nil
		},
	}
}

// newRespondersAddCmd grants answer access. --anyone makes the form
// anyone-with-link answerable; --to grants named responders (comma-separated).
// Named responders are created serially and non-atomically: a partial failure
// reports the per-address outcome and reruns are idempotent.
func (s *Service) newRespondersAddCmd(token string) *cobra.Command {
	var anyone bool
	var to string
	cmd := &cobra.Command{
		Use:   "add <form-id> (--anyone | --to a@b[,c@d])",
		Short: "Grant answer access (drive.file scope: only forms this assistant created)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			formID, err := extractFormID(args[0])
			if err != nil {
				return err
			}
			if err := validateResponderTarget(anyone, to); err != nil {
				return err
			}
			if anyone {
				return s.addAnyoneResponder(cmd, token, formID)
			}
			return s.addEmailResponders(cmd, token, formID, splitEmails(to))
		},
	}
	cmd.Flags().BoolVar(&anyone, "anyone", false, "let anyone with the link answer")
	cmd.Flags().StringVar(&to, "to", "", "comma-separated responder email(s)")
	return cmd
}

func (s *Service) addAnyoneResponder(cmd *cobra.Command, token, formID string) error {
	body, err := s.createPermission(cmd.Context(), token, formID, map[string]any{
		"view": "published", "role": "reader", "type": "anyone",
	})
	if err != nil {
		return err
	}
	if jsonOut(cmd) {
		return s.emit(body)
	}
	fmt.Fprintf(s.stdout(), "form %s is now answerable by anyone with the link\n", formID)
	return nil
}

func (s *Service) addEmailResponders(cmd *cobra.Command, token, formID string, emails []string) error {
	var added, failed []string
	for _, email := range emails {
		_, err := s.createPermission(cmd.Context(), token, formID, map[string]any{
			"view": "published", "role": "reader", "type": "user", "emailAddress": email,
		})
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", email, err))
			continue
		}
		added = append(added, email)
	}
	if len(failed) > 0 {
		return fmt.Errorf("forms: added %v; failed %v", added, failed)
	}
	if jsonOut(cmd) {
		return s.emitJSON(map[string]any{"formId": formID, "added": added})
	}
	fmt.Fprintf(s.stdout(), "granted answer access to %d responder(s): %s\n", len(added), strings.Join(added, ", "))
	return nil
}

// createPermission creates one Drive permission on the published view.
func (s *Service) createPermission(ctx context.Context, token, formID string, body map[string]any) ([]byte, error) {
	q := url.Values{"fields": {"id,type,role,emailAddress"}}
	return s.callDrive(ctx, token, http.MethodPost, "/files/"+url.PathEscape(formID)+"/permissions", q, body)
}

// newRespondersRemoveCmd revokes answer access. Removal finds the matching
// published-view permission and deletes it; a missing match is treated as
// already-removed (idempotent).
func (s *Service) newRespondersRemoveCmd(token string) *cobra.Command {
	var anyone bool
	var to string
	cmd := &cobra.Command{
		Use:   "remove <form-id> (--anyone | --to a@b)",
		Short: "Revoke answer access for anyone-with-link or a named responder",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			formID, err := extractFormID(args[0])
			if err != nil {
				return err
			}
			if err := validateResponderTarget(anyone, to); err != nil {
				return err
			}
			perms, err := s.listResponders(cmd.Context(), token, formID)
			if err != nil {
				return err
			}
			target := findResponder(perms, anyone, strings.TrimSpace(to))
			if target == nil {
				if jsonOut(cmd) {
					return s.emitJSON(map[string]any{"formId": formID, "status": "not-found", "removed": false})
				}
				fmt.Fprintln(s.stdout(), "responder not found (already removed)")
				return nil
			}
			if _, err := s.callDrive(cmd.Context(), token, http.MethodDelete, "/files/"+url.PathEscape(formID)+"/permissions/"+url.PathEscape(target.ID), nil, nil); err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitJSON(map[string]any{"formId": formID, "removed": true, "permissionId": target.ID})
			}
			fmt.Fprintf(s.stdout(), "removed responder %s from form %s\n", target.ID, formID)
			return nil
		},
	}
	cmd.Flags().BoolVar(&anyone, "anyone", false, "remove the anyone-with-link responder")
	cmd.Flags().StringVar(&to, "to", "", "responder email to remove")
	return cmd
}

// findResponder returns the published-view permission matching the target, or
// nil. anyone matches type=anyone; otherwise a case-insensitive email match.
func findResponder(perms []permission, anyone bool, email string) *permission {
	for i := range perms {
		p := &perms[i]
		if anyone && p.Type == "anyone" {
			return p
		}
		if !anyone && email != "" && strings.EqualFold(p.EmailAddress, email) {
			return p
		}
	}
	return nil
}

// validateResponderTarget enforces exactly one of --anyone / --to.
func validateResponderTarget(anyone bool, to string) error {
	to = strings.TrimSpace(to)
	if anyone && to != "" {
		return fmt.Errorf("forms: pass only one of --anyone or --to")
	}
	if !anyone && to == "" {
		return fmt.Errorf("forms: pass --anyone or --to <email>")
	}
	return nil
}

// splitEmails splits a comma-separated list, trimming spaces and dropping
// empties.
func splitEmails(to string) []string {
	parts := strings.Split(to, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if e := strings.TrimSpace(p); e != "" {
			out = append(out, e)
		}
	}
	return out
}
