package drive

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// folderMIME is the Drive mimeType for a folder; `mkdir` is a synthetic verb
// over files.create with this type (design 303).
const folderMIME = "application/vnd.google-apps.folder"

// listFields is the field mask requested for files.list — enough for a useful
// human table and a rich --json body.
const listFields = "nextPageToken,files(id,name,mimeType,modifiedTime,size,trashed,webViewLink,parents)"

// fileFields is the field mask for a single file: metadata, delivery link, and
// a sharing summary.
const fileFields = "id,name,mimeType,size,modifiedTime,trashed,parents,owners(displayName,emailAddress),webViewLink,webContentLink,shared,permissions(id,type,role,emailAddress,domain,displayName)"

// driveFile is the decoded shape shared by the human renderers.
type driveFile struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	MimeType     string `json:"mimeType"`
	Size         string `json:"size"`
	ModifiedTime string `json:"modifiedTime"`
	Trashed      bool   `json:"trashed"`
	WebViewLink  string `json:"webViewLink"`
}

func (s *Service) newAboutCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "about",
		Short:       "Show the connected account and storage quota (about.get)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("fields", "user(displayName,emailAddress),storageQuota(limit,usage,usageInDrive)")
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/about", q, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var a struct {
				User struct {
					DisplayName  string `json:"displayName"`
					EmailAddress string `json:"emailAddress"`
				} `json:"user"`
				StorageQuota struct {
					Limit        string `json:"limit"`
					Usage        string `json:"usage"`
					UsageInDrive string `json:"usageInDrive"`
				} `json:"storageQuota"`
			}
			if err := json.Unmarshal(body, &a); err != nil {
				return fmt.Errorf("drive: decode about: %w", err)
			}
			fmt.Fprintf(s.stdout(), "Account: %s <%s>\nUsage:   %s / %s (in Drive: %s)\n",
				a.User.DisplayName, a.User.EmailAddress,
				orDash(a.StorageQuota.Usage), quotaLimit(a.StorageQuota.Limit), orDash(a.StorageQuota.UsageInDrive))
			return nil
		},
	}
}

func (s *Service) newFilesListCmd(token string) *cobra.Command {
	var query, parent, pageToken string
	var max int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List files (native Drive query syntax via --query). Visible domain = files this tool created or the user granted it (drive.file).",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if max < 1 || max > 1000 {
				return fmt.Errorf("drive: --max must be between 1 and 1000 (Drive caps pageSize at 1000); got %d", max)
			}
			q := driveParams()
			q.Set("includeItemsFromAllDrives", "true")
			q.Set("fields", listFields)
			q.Set("pageSize", strconv.Itoa(max))
			clauses := []string{}
			if query != "" {
				clauses = append(clauses, query)
			}
			if parent != "" {
				clauses = append(clauses, "'"+parent+"' in parents")
			}
			// Drive's files.list returns trashed items too when the q clause
			// omits a trashed filter. Default to live files only, unless the
			// caller's --query already speaks about trashed state.
			if !strings.Contains(strings.ToLower(query), "trashed") {
				clauses = append(clauses, "trashed = false")
			}
			if len(clauses) > 0 {
				q.Set("q", strings.Join(clauses, " and "))
			}
			if pageToken != "" {
				q.Set("pageToken", pageToken)
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/files", q, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				Files         []driveFile `json:"files"`
				NextPageToken string      `json:"nextPageToken"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("drive: decode file list: %w", err)
			}
			if len(resp.Files) == 0 {
				fmt.Fprintln(s.stdout(), "no files (drive.file only sees files Helio created or the user granted it)")
				return nil
			}
			for _, f := range resp.Files {
				name := f.Name
				if f.Trashed {
					name += " (trashed)"
				}
				fmt.Fprintf(s.stdout(), "%s\t%s\t%s\n", f.ID, f.MimeType, name)
			}
			if resp.NextPageToken != "" {
				fmt.Fprintf(s.stdout(), "next page token: %s\n", resp.NextPageToken)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "Drive search query, e.g. name contains 'report' and mimeType='application/pdf' (passed through verbatim)")
	cmd.Flags().StringVar(&parent, "parent", "", "restrict to children of this folder id")
	cmd.Flags().IntVar(&max, "max", 20, "max results to return")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "page token from a previous list call")
	return cmd
}

func (s *Service) newFilesGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <file-id>",
		Short:       "Show file metadata: name, type, size, parents, owners, webViewLink, and a sharing summary",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.getFileMeta(cmd.Context(), token, args[0], fileFields)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var f struct {
				driveFile
				Parents []string `json:"parents"`
				Owners  []struct {
					DisplayName  string `json:"displayName"`
					EmailAddress string `json:"emailAddress"`
				} `json:"owners"`
				Shared      bool `json:"shared"`
				Permissions []struct {
					Role         string `json:"role"`
					Type         string `json:"type"`
					EmailAddress string `json:"emailAddress"`
					Domain       string `json:"domain"`
				} `json:"permissions"`
			}
			if err := json.Unmarshal(body, &f); err != nil {
				return fmt.Errorf("drive: decode file: %w", err)
			}
			fmt.Fprintf(s.stdout(),
				"Id:       %s\nName:     %s\nType:     %s\nSize:     %s\nModified: %s\nParents:  %s\nOwner:    %s\nLink:     %s\nShared:   %t\n",
				f.ID, f.Name, f.MimeType, orDash(f.Size), orDash(f.ModifiedTime),
				strings.Join(f.Parents, ", "), ownerLabel(f.Owners), f.WebViewLink, f.Shared)
			for _, p := range f.Permissions {
				fmt.Fprintf(s.stdout(), "  perm: %s\t%s\t%s\n", p.Role, p.Type, permTarget(p.EmailAddress, p.Domain))
			}
			return nil
		},
	}
}

func (s *Service) newFilesMkdirCmd(token string) *cobra.Command {
	var parent string
	cmd := &cobra.Command{
		Use:         "mkdir <name>",
		Short:       "Create a folder (synthetic: files.create with the folder mimeType)",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			meta := map[string]any{"name": args[0], "mimeType": folderMIME}
			if parent != "" {
				meta["parents"] = []string{parent}
			}
			q := driveParams()
			q.Set("fields", "id,name,mimeType,webViewLink")
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/files", q, meta)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var f driveFile
			if err := json.Unmarshal(body, &f); err != nil {
				return fmt.Errorf("drive: decode folder: %w", err)
			}
			fmt.Fprintf(s.stdout(), "created folder %s (%s)\n", f.Name, f.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&parent, "parent", "", "parent folder id (default: My Drive root)")
	return cmd
}

func (s *Service) newFilesUpdateCmd(token string) *cobra.Command {
	var name, addParent, removeParent, description string
	cmd := &cobra.Command{
		Use:         "update <file-id>",
		Short:       "Rename, move (--parent moves via addParents/removeParents), or set the description",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			meta := map[string]any{}
			if cmd.Flags().Changed("name") {
				meta["name"] = name
			}
			if cmd.Flags().Changed("description") {
				meta["description"] = description
			}
			q := driveParams()
			q.Set("fields", "id,name,parents,webViewLink")
			if addParent != "" {
				q.Set("addParents", addParent)
			}
			if removeParent != "" {
				q.Set("removeParents", removeParent)
			}
			if len(meta) == 0 && addParent == "" && removeParent == "" {
				return fmt.Errorf("drive: nothing to update — pass --name, --parent, --remove-parent, or --description")
			}
			body, err := s.call(cmd.Context(), token, http.MethodPatch, "/files/"+url.PathEscape(args[0]), q, meta)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var f driveFile
			if err := json.Unmarshal(body, &f); err != nil {
				return fmt.Errorf("drive: decode file: %w", err)
			}
			fmt.Fprintf(s.stdout(), "updated %s (%s)\n", orDash(f.Name), f.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "new name")
	cmd.Flags().StringVar(&addParent, "parent", "", "move into this folder id (added as a parent)")
	cmd.Flags().StringVar(&removeParent, "remove-parent", "", "remove this parent folder id (use with --parent to move)")
	cmd.Flags().StringVar(&description, "description", "", "new description")
	return cmd
}

func (s *Service) newFilesCopyCmd(token string) *cobra.Command {
	var name, parent string
	cmd := &cobra.Command{
		Use:         "copy <file-id>",
		Short:       "Copy a file (files.copy)",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			meta := map[string]any{}
			if cmd.Flags().Changed("name") {
				meta["name"] = name
			}
			if parent != "" {
				meta["parents"] = []string{parent}
			}
			q := driveParams()
			q.Set("fields", "id,name,mimeType,webViewLink")
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/files/"+url.PathEscape(args[0])+"/copy", q, meta)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var f driveFile
			if err := json.Unmarshal(body, &f); err != nil {
				return fmt.Errorf("drive: decode file: %w", err)
			}
			fmt.Fprintf(s.stdout(), "copied to %s (%s)\n", f.Name, f.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "name for the copy")
	cmd.Flags().StringVar(&parent, "parent", "", "destination folder id")
	return cmd
}

// newFilesTrashCmd builds trash (untrash=false) or untrash (untrash=true). Both
// are synthetic verbs over files.update setting trashed=true/false — trash is
// the only deletion path this tool exposes (permanent delete is intentionally
// omitted; design 303).
func (s *Service) newFilesTrashCmd(token string, untrash bool) *cobra.Command {
	verb, past, short := "trash", "trashed", "Move files to the trash (recoverable; the only deletion this tool exposes)"
	trashed := true
	if untrash {
		verb, past, short, trashed = "untrash", "untrashed", "Restore files from the trash", false
	}
	return &cobra.Command{
		Use:         verb + " <file-id>...",
		Short:       short,
		Args:        cobra.MinimumNArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := cleanIDs(args)
			if err != nil {
				return err
			}
			q := driveParams()
			q.Set("fields", "id,trashed")
			for _, id := range ids {
				if _, err := s.call(cmd.Context(), token, http.MethodPatch, "/files/"+url.PathEscape(id), q, map[string]any{"trashed": trashed}); err != nil {
					return err
				}
			}
			if jsonOut(cmd) {
				return s.emitJSON(map[string]any{"ids": ids, "status": past})
			}
			fmt.Fprintf(s.stdout(), "%s %d file(s)\n", past, len(ids))
			return nil
		},
	}
}

// getFileMeta fetches one file's metadata with the given field mask.
func (s *Service) getFileMeta(ctx context.Context, token, id, fields string) ([]byte, error) {
	q := driveParams()
	q.Set("fields", fields)
	return s.call(ctx, token, http.MethodGet, "/files/"+url.PathEscape(id), q, nil)
}
