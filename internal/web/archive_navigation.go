package web

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

type archiveAccount struct {
	ID, Name, Email, Host string
}

type archiveFolder struct {
	ID         int64
	Name, Path string
	Children   []*archiveFolder
}

type archivePage struct {
	Accounts                                          []archiveAccount
	Folders                                           []*archiveFolder
	Account, Scope, Search, Sender, From, Until, Next string
	Folder, Selected                                  int64
	Before                                            int64
	SearchFolders                                     []int64
	SearchOpen                                        bool
	Messages                                          []archivedMessage
	Preview                                           *archivePreview
}

type archivePreview struct {
	Message           archivedMessage
	Body, Note        string
	Parts             []attachment
	HasHTML, ShowHTML bool
}

func loadArchiveAccounts(ctx context.Context, db *sql.DB) ([]archiveAccount, error) {
	rows, err := db.QueryContext(ctx, `SELECT a.id,a.name,a.email,a.host FROM accounts a
 WHERE a.removed_at IS NULL OR EXISTS (SELECT 1 FROM folders f WHERE f.account_id=a.id)
 ORDER BY a.name,a.email,a.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []archiveAccount
	for rows.Next() {
		var a archiveAccount
		if err := rows.Scan(&a.ID, &a.Name, &a.Email, &a.Host); err != nil {
			return nil, err
		}
		values = append(values, a)
	}
	return values, rows.Err()
}

func loadArchiveFolders(ctx context.Context, db *sql.DB, account string) ([]*archiveFolder, error) {
	rows, err := db.QueryContext(ctx, "SELECT id,name,delimiter FROM folders WHERE account_id=? ORDER BY name COLLATE NOCASE,id", account)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roots []*archiveFolder
	nodes := map[string]*archiveFolder{}
	for rows.Next() {
		var id int64
		var path, delimiter string
		if err := rows.Scan(&id, &path, &delimiter); err != nil {
			return nil, err
		}
		parts := []string{path}
		if delimiter != "" && delimiter != "\x00" {
			parts = strings.Split(path, delimiter)
		}
		children := &roots
		for i, name := range parts {
			key := strings.Join(parts[:i+1], delimiter)
			node, exists := nodes[key]
			if !exists {
				node = &archiveFolder{Name: name, Path: key}
				nodes[key] = node
				*children = append(*children, node)
			}
			if i == len(parts)-1 {
				node.ID = id
			}
			children = &node.Children
		}
	}
	return roots, rows.Err()
}

func (p archivePage) values() url.Values {
	query := url.Values{"account": {p.Account}}
	if p.Folder != 0 {
		query.Set("folder", strconv.FormatInt(p.Folder, 10))
	}
	if p.Before > 0 {
		query.Set("before", strconv.FormatInt(p.Before, 10))
	}
	if p.SearchOpen {
		query.Set("search", "1")
	}
	if p.Scope != "" {
		query.Set("scope", p.Scope)
	}
	for name, value := range map[string]string{"q": p.Search, "sender": p.Sender, "from": p.From, "until": p.Until} {
		if value != "" {
			query.Set(name, value)
		}
	}
	for _, id := range p.SearchFolders {
		query.Add("in", strconv.FormatInt(id, 10))
	}
	return query
}

func (p archivePage) folderURL(folder int64) string {
	return "/archive?" + url.Values{"account": {p.Account}, "folder": {strconv.FormatInt(folder, 10)}}.Encode()
}

func (p archivePage) messageURL(id int64, html bool) string {
	query := p.values()
	query.Set("message", strconv.FormatInt(id, 10))
	if html {
		query.Set("view", "html")
	}
	return "/archive?" + query.Encode()
}

func (p archivePage) title() string {
	if p.Search != "" || p.Sender != "" || p.From != "" || p.Until != "" {
		switch p.Scope {
		case "all":
			return "Suche in allen Postfächern"
		case "folders":
			return "Suche in ausgewählten Ordnern"
		default:
			return "Suche im Postfach"
		}
	}
	if folder := findArchiveFolder(p.Folders, p.Folder); folder != nil {
		return folder.Path
	}
	return "Alle Ordner"
}

func findArchiveFolder(folders []*archiveFolder, id int64) *archiveFolder {
	if id == 0 {
		return nil
	}
	for _, folder := range folders {
		if folder.ID == id {
			return folder
		}
		if child := findArchiveFolder(folder.Children, id); child != nil {
			return child
		}
	}
	return nil
}

func archiveDate(value string) string {
	stamp, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return value
	}
	return stamp.Local().Format("02.01.2006 · 15:04")
}

func archiveSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	if size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
}

func archiveFolderChecked(page archivePage, id int64) bool {
	return slices.Contains(page.SearchFolders, id) || (page.Scope != "folders" && len(page.SearchFolders) == 0 && page.Folder == id)
}

func archiveCurrent(active bool) string {
	if active {
		return "page"
	}
	return "false"
}
