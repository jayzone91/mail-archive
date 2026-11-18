package web

import (
	"errors"
	"html"
	"io"
	"net/http"
	"strings"

	message "github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"
	htmlparse "golang.org/x/net/html"
)

var previewTags = map[string]bool{
	"p": true, "div": true, "span": true, "br": true, "hr": true, "strong": true, "b": true,
	"em": true, "i": true, "u": true, "blockquote": true, "pre": true, "code": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "ul": true, "ol": true, "li": true,
	"table": true, "thead": true, "tbody": true, "tr": true, "td": true, "th": true,
}

func safePreview(raw io.Reader) (string, error) {
	doc, err := htmlparse.Parse(io.LimitReader(raw, maxText))
	if err != nil {
		return "", err
	}
	var output strings.Builder
	var visit func(*htmlparse.Node, int)
	visit = func(node *htmlparse.Node, depth int) {
		if depth > 256 {
			return
		}
		if node.Type == htmlparse.TextNode {
			output.WriteString(html.EscapeString(node.Data))
			return
		}
		if node.Type == htmlparse.ElementNode {
			switch node.Data {
			case "head", "script", "style", "iframe", "object", "template", "svg", "math", "noscript":
				return
			}
		}
		safe := node.Type == htmlparse.ElementNode && node.Namespace == "" && previewTags[node.Data]
		if safe {
			output.WriteString("<" + node.Data + ">")
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child, depth+1)
		}
		if safe && node.Data != "br" && node.Data != "hr" {
			output.WriteString("</" + node.Data + ">")
		}
	}
	visit(doc, 0)
	return output.String(), nil
}

func (s *site) viewHTML(w http.ResponseWriter, r *http.Request) {
	item, ok := s.archivedMessage(w, r)
	if !ok {
		return
	}
	file, err := s.archiveStore().Open(item.Digest)
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
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			http.NotFound(w, r)
			return
		}
		if part == nil || (err != nil && !message.IsUnknownCharset(err)) {
			s.failure(w, err)
			return
		}
		header, ok := part.Header.(*mail.InlineHeader)
		if !ok {
			continue
		}
		contentType, _, _ := header.ContentType()
		if contentType != "text/html" {
			continue
		}
		preview, err := safePreview(part.Body)
		if err != nil {
			s.failure(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; frame-ancestors 'self'; form-action 'none'; base-uri 'none'")
		io.WriteString(w, "<!doctype html><html lang=\"de\"><meta charset=\"utf-8\"><title>Nachricht</title><body>"+preview+"</body></html>")
		return
	}
}
