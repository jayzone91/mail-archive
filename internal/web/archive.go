package web

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	protocol "github.com/emersion/go-imap/v2"
	"github.com/jayzone91/mail-archive/internal/archive"
)

type archivedMessage struct {
	ID                                                   int64
	Account, Folder, Date, Digest, Subject, From, To, CC string
	Size                                                 int64
}

func (s *site) archiveRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /archive", s.protect(s.listArchive))
	mux.HandleFunc("POST /archive/reindex", s.protect(s.reindexArchive))
	mux.HandleFunc("GET /archive/{id}", s.protect(s.viewMessage))
	mux.HandleFunc("GET /archive/{id}/eml", s.protect(s.downloadMessage))
	mux.HandleFunc("GET /archive/{id}/html", s.protect(s.viewHTML))
	mux.HandleFunc("GET /archive/{id}/parts/{part}", s.protect(s.downloadPart))
}

func (s *site) archiveStore() archive.Store {
	return s.archiveLocation.Store()
}

const archiveSelect = `SELECT messages.id, accounts.name, folders.name, messages.internal_date,
 messages.digest, messages.size, messages.envelope
 FROM messages JOIN folders ON folders.id=messages.folder_id JOIN accounts ON accounts.id=folders.account_id`

func readArchivedMessage(row interface{ Scan(...any) error }) (archivedMessage, error) {
	var message archivedMessage
	var data string
	err := row.Scan(&message.ID, &message.Account, &message.Folder, &message.Date, &message.Digest, &message.Size, &data)
	if err != nil {
		return message, err
	}
	var envelope protocol.Envelope
	if err := json.Unmarshal([]byte(data), &envelope); err != nil {
		return message, err
	}
	message.Subject = envelope.Subject
	message.From = archive.Addresses(envelope.From)
	message.To = archive.Addresses(envelope.To)
	message.CC = archive.Addresses(envelope.Cc)
	return message, nil
}

func (s *site) reindexArchive(w http.ResponseWriter, r *http.Request) {
	if err := archive.Reindex(r.Context(), s.db); err != nil {
		s.failure(w, err)
		return
	}
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (s *site) listArchive(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	accounts, err := loadArchiveAccounts(r.Context(), s.db)
	if err != nil {
		s.failure(w, err)
		return
	}
	page := archivePage{Accounts: accounts, Account: query.Get("account"), Scope: query.Get("scope"), SearchOpen: query.Get("search") == "1",
		Search: strings.TrimSpace(query.Get("q")), Sender: strings.TrimSpace(query.Get("sender")), From: query.Get("from"), Until: query.Get("until")}
	if len(accounts) == 0 {
		s.render(w, r, http.StatusOK, layout("Archiv", true, archiveView(page)))
		return
	}
	var found bool
	for _, account := range accounts {
		if account.ID == page.Account {
			found = true
			break
		}
	}
	if !found {
		page.Account = accounts[0].ID
	}
	page.Folders, err = loadArchiveFolders(r.Context(), s.db, page.Account)
	if err != nil {
		s.failure(w, err)
		return
	}
	page.Folder, _ = strconv.ParseInt(query.Get("folder"), 10, 64)
	if findArchiveFolder(page.Folders, page.Folder) == nil {
		page.Folder = 0
	}
	page.Before, _ = strconv.ParseInt(query.Get("before"), 10, 64)
	page.Selected, _ = strconv.ParseInt(query.Get("message"), 10, 64)
	searching := page.Search != "" || page.Sender != "" || page.From != "" || page.Until != ""
	page.SearchOpen = page.SearchOpen || searching
	if page.Scope != "all" && page.Scope != "folders" {
		page.Scope = "account"
	}
	for _, value := range query["in"] {
		id, _ := strconv.ParseInt(value, 10, 64)
		if findArchiveFolder(page.Folders, id) != nil {
			page.SearchFolders = append(page.SearchFolders, id)
		}
	}
	for _, date := range []string{page.From, page.Until} {
		if date != "" {
			if _, err := time.Parse(time.DateOnly, date); err != nil {
				http.Error(w, "Ungültiger Datumsfilter.", http.StatusBadRequest)
				return
			}
		}
	}
	account, folder := page.Account, page.Folder
	scopeFolders := false
	if searching {
		folder = 0
		if page.Scope == "all" {
			account = ""
		}
		scopeFolders = page.Scope == "folders"
	}
	folders, _ := json.Marshal(page.SearchFolders)
	match := `"` + strings.ReplaceAll(page.Search, `"`, `""`) + `"`
	rows, err := s.db.QueryContext(r.Context(), archiveSelect+`
 WHERE (?='' OR accounts.id=?) AND (?=0 OR folders.id=?) AND (?<=0 OR messages.id<?)
 AND (?=0 OR folders.id IN (SELECT value FROM json_each(?)))
 AND (?='' OR substr(messages.internal_date,1,10)>=?) AND (?='' OR substr(messages.internal_date,1,10)<=?)
 AND (?='' OR messages.id IN (SELECT rowid FROM archive_search WHERE archive_search MATCH ?))
 AND (?='' OR EXISTS(SELECT 1 FROM json_each(messages.envelope,'$.From') AS sender
 WHERE instr(lower(COALESCE(json_extract(sender.value,'$.Name'),'') || ' ' ||
 COALESCE(json_extract(sender.value,'$.Mailbox'),'') || '@' ||
 COALESCE(json_extract(sender.value,'$.Host'),'')),lower(?))>0))
 ORDER BY messages.id DESC LIMIT 101`, account, account, folder, folder, page.Before, page.Before, scopeFolders, string(folders),
		page.From, page.From, page.Until, page.Until, page.Search, match, page.Sender, page.Sender)
	if err != nil {
		s.failure(w, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		message, err := readArchivedMessage(rows)
		if err != nil {
			s.failure(w, err)
			return
		}
		page.Messages = append(page.Messages, message)
	}
	if err := rows.Err(); err != nil {
		s.failure(w, err)
		return
	}
	rows.Close()
	if len(page.Messages) > 100 {
		page.Messages = page.Messages[:100]
		link := page.values()
		link.Set("before", strconv.FormatInt(page.Messages[99].ID, 10))
		page.Next = "/archive?" + link.Encode()
	}
	for _, message := range page.Messages {
		if message.ID == page.Selected {
			preview, err := s.loadArchivePreview(w, message, query.Get("view") == "html")
			if err != nil {
				s.failure(w, err)
				return
			}
			page.Preview = &preview
			break
		}
	}
	s.render(w, r, http.StatusOK, layout("Archiv", true, archiveView(page)))
}

func (s *site) archivedMessage(w http.ResponseWriter, r *http.Request) (archivedMessage, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return archivedMessage{}, false
	}
	message, err := readArchivedMessage(s.db.QueryRowContext(r.Context(), archiveSelect+" WHERE messages.id=?", id))
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return message, false
	}
	if err != nil {
		s.failure(w, err)
		return message, false
	}
	return message, true
}

func (s *site) viewMessage(w http.ResponseWriter, r *http.Request) {
	message, ok := s.archivedMessage(w, r)
	if !ok {
		return
	}
	preview, err := s.loadArchivePreview(w, message, r.URL.Query().Get("view") == "html")
	if err != nil {
		s.failure(w, err)
		return
	}
	s.render(w, r, http.StatusOK, layout("Archivierte Nachricht", true, messageView(preview, archiveURL(message.ID), archiveURL(message.ID)+"?view=html", "/archive")))
}

func (s *site) loadArchivePreview(w http.ResponseWriter, message archivedMessage, showHTML bool) (archivePreview, error) {
	file, err := s.archiveStore().Open(message.Digest)
	if err != nil {
		return archivePreview{}, err
	}
	defer file.Close()
	body, parts, hasHTML, err := readMIME(file)
	note := ""
	if err != nil {
		note = "Die MIME-Anzeige ist unvollständig. Die Originaldatei kann heruntergeladen werden."
	}
	showHTML = hasHTML && showHTML
	if showHTML {
		w.Header().Set("Content-Security-Policy", webPolicy+"; frame-src 'self'")
	}
	return archivePreview{Message: message, Body: body, Parts: parts, HasHTML: hasHTML, ShowHTML: showHTML, Note: note}, nil
}

func (s *site) downloadMessage(w http.ResponseWriter, r *http.Request) {
	message, ok := s.archivedMessage(w, r)
	if !ok {
		return
	}
	file, err := s.archiveStore().Open(message.Digest)
	if err != nil {
		s.failure(w, err)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": fmt.Sprintf("mail-%d.eml", message.ID)}))
	http.ServeContent(w, r, "message.eml", timeZero, file)
}

func archiveURL(id int64) string { return "/archive/" + strconv.FormatInt(id, 10) }
