package microsoftonedrive

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// itemList is a Graph collection response of DriveItems.
type itemList struct {
	Value    []driveItem `json:"value"`
	NextLink string      `json:"@odata.nextLink"`
}

// fetchCollection GETs a DriveItem collection, following an explicit
// @odata.nextLink (page) when supplied and otherwise applying $top (max).
func (s *Service) fetchCollection(cmd *cobra.Command, token, path string, max int, page string) ([]byte, error) {
	if page != "" {
		return s.callEndpoint(cmd.Context(), token, http.MethodGet, page, page, "", nil)
	}
	endpoint := s.base() + path
	if strings.Contains(path, "?") {
		endpoint += "&$top=" + strconv.Itoa(max)
	} else {
		endpoint += "?$top=" + strconv.Itoa(max)
	}
	return s.callEndpoint(cmd.Context(), token, http.MethodGet, path, endpoint, "", nil)
}

// renderItemList prints a collection body as a human summary or raw JSON.
func (s *Service) renderItemList(cmd *cobra.Command, body []byte) error {
	if jsonOut(cmd) {
		return s.emit(body)
	}
	var list itemList
	if err := json.Unmarshal(body, &list); err != nil {
		return fmt.Errorf("microsoft-onedrive: decode item list: %w", err)
	}
	if len(list.Value) == 0 {
		fmt.Fprintln(s.stdout(), "no items")
		return nil
	}
	for _, it := range list.Value {
		if it.Folder != nil {
			fmt.Fprintf(s.stdout(), "%s\t%s/\t(%d items)\t%s\n", it.ID, it.Name, it.Folder.ChildCount, it.LastModifiedDateTime)
			continue
		}
		fmt.Fprintf(s.stdout(), "%s\t%s\t%d bytes\t%s\t%s\n", it.ID, it.Name, it.Size, it.mimeType(), it.LastModifiedDateTime)
	}
	if list.NextLink != "" {
		fmt.Fprintf(s.stdout(), "next page: %s\n", list.NextLink)
	}
	return nil
}

func (s *Service) newItemsListCmd(token string) *cobra.Command {
	var path, parent, page string
	var max int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List a folder's children (drive root by default; --path or --parent to target a folder)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			resource, err := childrenResource(parent, path)
			if err != nil {
				return err
			}
			body, err := s.fetchCollection(cmd, token, resource, max, page)
			if err != nil {
				return err
			}
			return s.renderItemList(cmd, body)
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "root-relative folder path to list")
	cmd.Flags().StringVar(&parent, "parent", "", "folder item id to list")
	cmd.Flags().IntVar(&max, "max", 20, "max results to return")
	cmd.Flags().StringVar(&page, "page", "", "@odata.nextLink from a previous list call")
	cmd.MarkFlagsMutuallyExclusive("path", "parent")
	return cmd
}

func (s *Service) newItemsGetCmd(token string) *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:         "get [item-id]",
		Short:       "Show one item's metadata (name / size / mimeType / lastModified)",
		Args:        cobra.MaximumNArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			id := firstArg(args)
			resource, err := itemResource(id, path)
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, resource, nil, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var it driveItem
			if err := json.Unmarshal(body, &it); err != nil {
				return fmt.Errorf("microsoft-onedrive: decode item: %w", err)
			}
			fmt.Fprintf(s.stdout(), "Id:       %s\nName:     %s\nKind:     %s\n", it.ID, it.Name, it.kind())
			if it.Folder != nil {
				fmt.Fprintf(s.stdout(), "Children: %d\n", it.Folder.ChildCount)
			} else {
				fmt.Fprintf(s.stdout(), "Size:     %d bytes\nMimeType: %s\n", it.Size, it.mimeType())
			}
			fmt.Fprintf(s.stdout(), "Modified: %s\nWebUrl:   %s\n", it.LastModifiedDateTime, it.WebURL)
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "root-relative item path instead of an id")
	return cmd
}

func (s *Service) newItemsMkdirCmd(token string) *cobra.Command {
	var name, parent, path string
	cmd := &cobra.Command{
		Use:         "mkdir",
		Short:       "Create a folder (in the drive root by default; --parent or --path to nest)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("microsoft-onedrive: --name is required")
			}
			resource, err := childrenResource(parent, path)
			if err != nil {
				return err
			}
			payload := map[string]any{
				"name":                              name,
				"folder":                            map[string]any{},
				"@microsoft.graph.conflictBehavior": "fail",
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, resource, nil, payload)
			if err != nil {
				return err
			}
			return s.renderItemResult(cmd, body, "created folder")
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "folder name to create")
	cmd.Flags().StringVar(&parent, "parent", "", "parent folder item id")
	cmd.Flags().StringVar(&path, "path", "", "root-relative parent folder path")
	_ = cmd.MarkFlagRequired("name")
	cmd.MarkFlagsMutuallyExclusive("parent", "path")
	return cmd
}

func (s *Service) newItemsMoveCmd(token string) *cobra.Command {
	var toDir, name string
	cmd := &cobra.Command{
		Use:         "move <item-id>",
		Short:       "Move an item into another folder (optionally renaming it)",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(toDir) == "" {
				return fmt.Errorf("microsoft-onedrive: --to <dir-id> is required")
			}
			payload := map[string]any{"parentReference": map[string]any{"id": toDir}}
			if name != "" {
				payload["name"] = name
			}
			resource, err := itemResource(args[0], "")
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodPatch, resource, nil, payload)
			if err != nil {
				return err
			}
			return s.renderItemResult(cmd, body, "moved item")
		},
	}
	cmd.Flags().StringVar(&toDir, "to", "", "destination folder item id")
	cmd.Flags().StringVar(&name, "name", "", "new name for the item (optional)")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}

func (s *Service) newItemsRenameCmd(token string) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:         "rename <item-id>",
		Short:       "Rename an item in place",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("microsoft-onedrive: --name is required")
			}
			resource, err := itemResource(args[0], "")
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodPatch, resource, nil, map[string]any{"name": name})
			if err != nil {
				return err
			}
			return s.renderItemResult(cmd, body, "renamed item")
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "new item name")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func (s *Service) newItemsShareCmd(token string) *cobra.Command {
	var linkType, scope string
	cmd := &cobra.Command{
		Use:         "share <item-id>",
		Short:       "Create a sharing link (default scope organization — anonymous exposes the item publicly)",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if linkType != "view" && linkType != "edit" {
				return fmt.Errorf("microsoft-onedrive: --type must be view or edit, got %q", linkType)
			}
			if scope != "anonymous" && scope != "organization" {
				return fmt.Errorf("microsoft-onedrive: --scope must be anonymous or organization, got %q", scope)
			}
			resource, err := itemResource(args[0], "")
			if err != nil {
				return err
			}
			payload := map[string]any{"type": linkType, "scope": scope}
			body, err := s.call(cmd.Context(), token, http.MethodPost, resource+"/createLink", nil, payload)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				Link struct {
					WebURL string `json:"webUrl"`
					Type   string `json:"type"`
					Scope  string `json:"scope"`
				} `json:"link"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("microsoft-onedrive: decode sharing link: %w", err)
			}
			fmt.Fprintf(s.stdout(), "sharing link (%s, %s): %s\n", resp.Link.Type, resp.Link.Scope, resp.Link.WebURL)
			return nil
		},
	}
	cmd.Flags().StringVar(&linkType, "type", "view", "link type: view or edit")
	cmd.Flags().StringVar(&scope, "scope", "organization", "link scope: organization or anonymous")
	return cmd
}

func (s *Service) newItemsDeleteCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "delete <item-id>...",
		Short:       "Move items to the recycle bin",
		Args:        cobra.MinimumNArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := cleanItemIDs(args)
			if err != nil {
				return err
			}
			// Deletes are applied one item at a time and stop on the first
			// failure; surface the items already recycled so partial progress is
			// visible to the agent rather than silently lost.
			deleted := make([]string, 0, len(ids))
			for _, id := range ids {
				resource, err := itemResource(id, "")
				if err != nil {
					return err
				}
				if _, err := s.call(cmd.Context(), token, http.MethodDelete, resource, nil, nil); err != nil {
					if len(deleted) > 0 {
						return fmt.Errorf("microsoft-onedrive: deleted %v before failing on %q: %w", deleted, id, err)
					}
					return err
				}
				deleted = append(deleted, id)
			}
			if jsonOut(cmd) {
				return s.emitJSON(map[string]any{"ids": ids, "status": "deleted"})
			}
			fmt.Fprintf(s.stdout(), "deleted %d item(s)\n", len(ids))
			return nil
		},
	}
}

// renderItemResult prints a DriveItem create/update response.
func (s *Service) renderItemResult(cmd *cobra.Command, body []byte, verb string) error {
	if jsonOut(cmd) {
		return s.emit(body)
	}
	var it driveItem
	if err := json.Unmarshal(body, &it); err != nil {
		return fmt.Errorf("microsoft-onedrive: decode item: %w", err)
	}
	fmt.Fprintf(s.stdout(), "%s %s (id %s)\n", verb, it.Name, it.ID)
	return nil
}

// firstArg returns the first positional arg, or "" when none were given.
func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return strings.TrimSpace(args[0])
}

// cleanItemIDs splits every multi-id arg on whitespace and drops empties, so a
// pipeline that pastes several ids into one arg still works.
func cleanItemIDs(args []string) ([]string, error) {
	ids := make([]string, 0, len(args))
	for _, arg := range args {
		ids = append(ids, strings.Fields(arg)...)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("microsoft-onedrive: no valid item ids")
	}
	return ids, nil
}
