package archive

import (
	"errors"
	"io"
	"strings"

	message "github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset"
	"github.com/emersion/go-message/mail"
	"golang.org/x/net/html"
)

const previewLimit = 1024 * 1024

// SearchText extracts visible message text without interpreting or fetching links.
func SearchText(source io.Reader) string {
	reader, _ := mail.CreateReader(source)
	if reader == nil {
		return ""
	}
	defer reader.Close()
	var plain, fallback strings.Builder
	for {
		part, err := reader.NextPart()
		if (err != nil && !message.IsUnknownCharset(err)) || part == nil {
			break
		}
		header, ok := part.Header.(*mail.InlineHeader)
		if !ok {
			continue
		}
		contentType, _, _ := header.ContentType()
		switch contentType {
		case "text/plain":
			if plain.Len() < previewLimit {
				io.Copy(&plain, io.LimitReader(part.Body, int64(previewLimit-plain.Len())))
			}
		case "text/html":
			if fallback.Len() == 0 {
				fallback.WriteString(HTMLText(part.Body))
			}
		}
	}
	if plain.Len() != 0 {
		return strings.ToValidUTF8(plain.String(), "�")
	}
	return strings.ToValidUTF8(fallback.String(), "�")
}

func HTMLText(source io.Reader) string {
	tokenizer := html.NewTokenizer(io.LimitReader(source, previewLimit))
	var result strings.Builder
	ignore := ""
	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			if errors.Is(tokenizer.Err(), io.EOF) {
				return result.String()
			}
			return result.String()
		case html.StartTagToken:
			tag, _ := tokenizer.TagName()
			name := string(tag)
			if ignore == "" && (name == "script" || name == "style" || name == "title" || name == "noscript") {
				ignore = name
			}
			if ignore == "" && (name == "p" || name == "br" || name == "li" || name == "div") {
				result.WriteByte('\n')
			}
		case html.EndTagToken:
			tag, _ := tokenizer.TagName()
			if string(tag) == ignore {
				ignore = ""
			}
		case html.TextToken:
			if ignore == "" {
				result.Write(tokenizer.Text())
			}
		}
	}
}
