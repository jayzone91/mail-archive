package web

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	message "github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset"
	"github.com/emersion/go-message/mail"
	"github.com/jayzone91/mail-archive/internal/archive"
)

const maxText = 1024 * 1024

var timeZero time.Time

type attachment struct {
	Index int
	Name  string
}

func readMIME(source io.Reader) (string, []attachment, bool, error) {
	reader, err := mail.CreateReader(source)
	if reader == nil {
		return "", nil, false, err
	}
	defer reader.Close()
	var body strings.Builder
	var fallback string
	var hasHTML bool
	var attachments []attachment
	for index := 1; ; index++ {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil && !message.IsUnknownCharset(err) {
			return body.String(), attachments, hasHTML, err
		}
		if part == nil {
			return body.String(), attachments, hasHTML, err
		}
		switch header := part.Header.(type) {
		case *mail.InlineHeader:
			contentType, _, _ := header.ContentType()
			if contentType == "text/plain" && body.Len() < maxText {
				if body.Len() > 0 {
					body.WriteByte('\n')
				}
				if _, err := io.Copy(&body, io.LimitReader(part.Body, int64(maxText-body.Len()))); err != nil {
					return body.String(), attachments, hasHTML, err
				}
			}
			if contentType == "text/html" && !hasHTML {
				hasHTML = true
				fallback = archive.HTMLText(part.Body)
			}
		case *mail.AttachmentHeader:
			filename, _ := header.Filename()
			attachments = append(attachments, attachment{Index: index, Name: attachmentName(filename, index)})
		}
	}
	if body.Len() == 0 {
		return fallback, attachments, hasHTML, nil
	}
	return body.String(), attachments, hasHTML, nil
}

func attachmentName(filename string, index int) string {
	filename = strings.ReplaceAll(filename, "\\", "/")
	if last := strings.LastIndexByte(filename, '/'); last >= 0 {
		filename = filename[last+1:]
	}
	filename = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, filename)
	if filename == "" || filename == "." || filename == ".." {
		return fmt.Sprintf("anhang-%d", index)
	}
	return filename
}

func (s *site) downloadPart(w http.ResponseWriter, r *http.Request) {
	messageID, ok := s.archivedMessage(w, r)
	if !ok {
		return
	}
	wanted, err := strconv.Atoi(r.PathValue("part"))
	if err != nil || wanted < 1 {
		http.NotFound(w, r)
		return
	}
	file, err := s.archiveStore().Open(messageID.Digest)
	if err != nil {
		s.failure(w, err)
		return
	}
	defer file.Close()
	reader, err := mail.CreateReader(file)
	if reader == nil {
		s.failure(w, err)
		return
	}
	defer reader.Close()
	for index := 1; index <= wanted; index++ {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			http.NotFound(w, r)
			return
		}
		if part == nil || (err != nil && !message.IsUnknownCharset(err)) {
			s.failure(w, err)
			return
		}
		if index != wanted {
			continue
		}
		header, ok := part.Header.(*mail.AttachmentHeader)
		if !ok {
			http.NotFound(w, r)
			return
		}
		filename, _ := header.Filename()
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": attachmentName(filename, index)}))
		if _, err := io.Copy(w, part.Body); err != nil {
			s.log.Warn("Anhangdownload abgebrochen", "message", messageID.ID, "error", err)
		}
	}
}
