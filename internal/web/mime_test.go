package web

import (
	"strings"
	"testing"
)

func TestMIMEBodyAndAttachment(t *testing.T) {
	raw := strings.Join([]string{
		"MIME-Version: 1.0",
		"Content-Type: multipart/mixed; boundary=synthetic",
		"",
		"--synthetic",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Testnachricht",
		"--synthetic",
		"Content-Type: text/html; charset=utf-8",
		"",
		"<script>untrusted()</script>",
		"--synthetic",
		"Content-Type: text/plain",
		"Content-Disposition: attachment; filename=\"../fixture.txt\"",
		"",
		"Inhalt",
		"--synthetic--",
		"",
	}, "\r\n")
	body, parts, hasHTML, err := readMIME(strings.NewReader(raw))
	if err != nil || !hasHTML || body != "Testnachricht" || len(parts) != 1 || parts[0].Name != "fixture.txt" || parts[0].Index != 3 {
		t.Fatalf("MIME-Anzeige: %q, %+v, %v", body, parts, err)
	}
}

func TestSafePreview(t *testing.T) {
	preview, err := safePreview(strings.NewReader(`<p onclick="bad()">Hallo <strong>Welt</strong></p><img src="https://example.test/tracker"><script>bad()</script><a href="https://example.test/">Link</a><svg><foreignObject><p>Fremd</p></foreignObject></svg>`))
	if err != nil || preview != "<p>Hallo <strong>Welt</strong></p>Link" {
		t.Fatalf("HTML-Vorschau: %q, %v", preview, err)
	}
}
