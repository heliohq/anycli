package microsoftonedrive

import (
	"fmt"
	"net/url"
	"strings"
)

// driveItem is the subset of a Microsoft Graph DriveItem the human summaries
// render. The raw body is passed through untouched under --json.
type driveItem struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	Size                 int64  `json:"size"`
	WebURL               string `json:"webUrl"`
	LastModifiedDateTime string `json:"lastModifiedDateTime"`
	Folder               *struct {
		ChildCount int `json:"childCount"`
	} `json:"folder"`
	File *struct {
		MimeType string `json:"mimeType"`
	} `json:"file"`
}

// kind reports "folder" or "file" for one-line listings.
func (d driveItem) kind() string {
	if d.Folder != nil {
		return "folder"
	}
	return "file"
}

// mimeType returns the file's MIME type, or "" for folders / unknown.
func (d driveItem) mimeType() string {
	if d.File != nil {
		return d.File.MimeType
	}
	return ""
}

// escapeDrivePath URL-escapes each segment of a root-relative OneDrive path,
// preserving the "/" separators so Graph's /me/drive/root:/a/b:/ addressing
// stays intact. Leading/trailing slashes are trimmed.
func escapeDrivePath(p string) string {
	p = strings.Trim(strings.TrimSpace(p), "/")
	if p == "" {
		return ""
	}
	parts := strings.Split(p, "/")
	for i, seg := range parts {
		parts[i] = url.PathEscape(seg)
	}
	return strings.Join(parts, "/")
}

// itemResource returns the Graph resource path for an item addressed by id or
// by root-relative path. Exactly one of id / path must be set.
func itemResource(id, path string) (string, error) {
	switch {
	case id != "" && path != "":
		return "", fmt.Errorf("microsoft-onedrive: pass either an item id or --path, not both")
	case id != "":
		return "/me/drive/items/" + url.PathEscape(id), nil
	case strings.TrimSpace(path) != "":
		return "/me/drive/root:/" + escapeDrivePath(path) + ":", nil
	default:
		return "", fmt.Errorf("microsoft-onedrive: an item id or --path is required")
	}
}

// childrenResource returns the Graph children collection for a container
// addressed by id, by root-relative path, or (both empty) the drive root.
func childrenResource(id, path string) (string, error) {
	switch {
	case id != "" && path != "":
		return "", fmt.Errorf("microsoft-onedrive: pass either --parent id or --path, not both")
	case id != "":
		return "/me/drive/items/" + url.PathEscape(id) + "/children", nil
	case strings.TrimSpace(path) != "":
		return "/me/drive/root:/" + escapeDrivePath(path) + ":/children", nil
	default:
		return "/me/drive/root/children", nil
	}
}

// uploadTargetResource returns the Graph resource path addressing a
// destination file <name> inside a parent folder (by id, by root-relative
// path, or the drive root). The caller appends /content or
// /createUploadSession.
func uploadTargetResource(parentID, parentPath, name string) (string, error) {
	if parentID != "" && strings.TrimSpace(parentPath) != "" {
		return "", fmt.Errorf("microsoft-onedrive: pass either --parent id or --to path, not both")
	}
	escName := url.PathEscape(name)
	switch {
	case parentID != "":
		return "/me/drive/items/" + url.PathEscape(parentID) + ":/" + escName + ":", nil
	case strings.TrimSpace(parentPath) != "":
		return "/me/drive/root:/" + escapeDrivePath(parentPath) + "/" + escName + ":", nil
	default:
		return "/me/drive/root:/" + escName + ":", nil
	}
}

// searchResource builds the Graph drive search function path for a query. The
// query passes through verbatim; a single quote is OData-escaped by doubling.
func searchResource(query string) string {
	odata := strings.ReplaceAll(query, "'", "''")
	// PathEscape keeps the value inside the q='...' literal URL-safe while
	// leaving the surrounding function syntax untouched.
	escaped := strings.ReplaceAll(url.PathEscape(odata), "+", "%20")
	return "/me/drive/root/search(q='" + escaped + "')"
}
