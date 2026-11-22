package smtp

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"net/textproto"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// TestBuildMIMESimple verifica i header esatti del caso senza allegati:
// From/To/Cc/Subject/Date/Message-ID/MIME-Version nell'ordine previsto,
// nessun Bcc anche quando presente in msg.Outgoing, corpo text/plain
// con CRLF.
func TestBuildMIMESimple(t *testing.T) {
	now := time.Date(2026, 11, 21, 10, 30, 0, 0, time.UTC)
	m := msg.Outgoing{
		From:     "me@example.com",
		To:       []string{"a@b.com", "c@d.com"},
		Cc:       []string{"e@f.com"},
		Bcc:      []string{"segreto@g.com"},
		Subject:  "Salve",
		BodyText: "riga1\nriga2\n",
	}
	out, err := buildMIME(m, now)
	if err != nil {
		t.Fatalf("buildMIME: %v", err)
	}
	s := string(out)

	// Bcc MAI negli header.
	if strings.Contains(s, "Bcc") {
		t.Errorf("buildMIME contiene Bcc (proibito):\n%s", s)
	}

	wantHeaders := []string{
		"From: me@example.com\r\n",
		"To: a@b.com, c@d.com\r\n",
		"Cc: e@f.com\r\n",
		"Subject: Salve\r\n",
		"Date: " + now.Format(time.RFC1123Z) + "\r\n",
		"MIME-Version: 1.0\r\n",
		"Content-Type: text/plain; charset=utf-8\r\n",
	}
	for _, h := range wantHeaders {
		if !strings.Contains(s, h) {
			t.Errorf("header mancante %q in:\n%s", h, s)
		}
	}

	// Corpo: LF convertito in CRLF.
	if !strings.Contains(s, "\r\nriga1\r\nriga2\r\n") {
		t.Errorf("corpo non CRLF in:\n%s", s)
	}

	// Message-ID: <hex16@dominio-di-From>.
	re := regexp.MustCompile(`Message-ID: <([0-9a-f]{16})@example\.com>\r\n`)
	if re.FindString(s) == "" {
		t.Errorf("Message-ID non conforme a <hex16@example.com> in:\n%s", s)
	}
}

// TestBuildMIMENonASCIISubject: subject con caratteri non-ASCII → Q-encoded.
func TestBuildMIMENonASCIISubject(t *testing.T) {
	now := time.Date(2026, 11, 21, 10, 30, 0, 0, time.UTC)
	m := msg.Outgoing{
		From:     "me@example.com",
		To:       []string{"a@b.com"},
		Subject:  "Caffè à Parigi",
		BodyText: "ciao",
	}
	out, err := buildMIME(m, now)
	if err != nil {
		t.Fatalf("buildMIME: %v", err)
	}
	s := string(out)

	if !strings.Contains(s, "Subject: =?utf-8?q?") {
		t.Errorf("Subject non Q-encoded in:\n%s", s)
	}

	// Il subject Q-encoded deve decodificare al valore originale.
	h := headerValue(s, "Subject")
	dec := new(mime.WordDecoder)
	got, err := dec.DecodeHeader(h)
	if err != nil {
		t.Fatalf("DecodeHeader(%q): %v", h, err)
	}
	if got != "Caffè à Parigi" {
		t.Errorf("Subject decodificato = %q, want %q", got, "Caffè à Parigi")
	}
}

// TestBuildMIMEAttachment: con allegati → multipart/mixed, prima parte
// text/plain, poi allegati con filename, Content-Type da estensione
// (fallback octet-stream) e base64 a righe 76 CRLF che decodifica al
// contenuto originale.
func TestBuildMIMEAttachment(t *testing.T) {
	dir := t.TempDir()
	txtPath := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(txtPath, []byte("contenuto allegato"), 0o644); err != nil {
		t.Fatal(err)
	}
	binPath := filepath.Join(dir, "blob")
	bin := []byte("binario\x00\x01\xff\x02fine")
	if err := os.WriteFile(binPath, bin, 0o644); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 11, 21, 10, 30, 0, 0, time.UTC)
	m := msg.Outgoing{
		From:        "me@example.com",
		To:          []string{"a@b.com"},
		Subject:     "Con allegati",
		BodyText:    "corpo\nmultiriga",
		Attachments: []string{txtPath, binPath},
	}
	out, err := buildMIME(m, now)
	if err != nil {
		t.Fatalf("buildMIME: %v", err)
	}
	s := string(out)

	// Multipart/mixed con boundary.
	matches := regexp.MustCompile(`Content-Type: multipart/mixed; boundary=(\S+)\r\n`).FindStringSubmatch(s)
	if matches == nil {
		t.Fatalf("multipart/mixed con boundary assente in:\n%s", s)
	}
	boundary := matches[1]

	// Parse del multipart (contenuti letti dentro il loop: lo streaming
	// del reader non permette di rileggerli dopo NextPart).
	mr := multipart.NewReader(bytes.NewReader(out), boundary)
	type part struct {
		header textproto.MIMEHeader
		body   []byte
	}
	var parts []part
	for {
		p, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("NextPart: %v", err)
		}
		body, err := io.ReadAll(p)
		if err != nil {
			t.Fatalf("read part %q: %v", p.Header.Get("Content-Disposition"), err)
		}
		parts = append(parts, part{header: p.Header, body: body})
	}
	if len(parts) != 3 {
		t.Fatalf("parti = %d, want 3 (testo + 2 allegati)", len(parts))
	}

	// Prima parte: text/plain.
	if ct := parts[0].header.Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("prima parte Content-Type = %q, want text/plain; charset=utf-8", ct)
	}
	if !bytes.Contains(parts[0].body, []byte("corpo\r\nmultiriga")) {
		t.Errorf("prima parte corpo = %q, want CRLF", parts[0].body)
	}

	// Seconda parte: notes.txt, Content-Type da estensione, base64.
	a := parts[1]
	if a.header.Get("Content-Disposition") != `attachment; filename="notes.txt"` {
		t.Errorf("Content-Disposition = %q, want attachment; filename=\"notes.txt\"", a.header.Get("Content-Disposition"))
	}
	if ct := a.header.Get("Content-Type"); ct != mime.TypeByExtension(".txt") {
		t.Errorf("allegato .txt Content-Type = %q, want mime.TypeByExtension(.txt) = %q", ct, mime.TypeByExtension(".txt"))
	}
	if cte := a.header.Get("Content-Transfer-Encoding"); cte != "base64" {
		t.Errorf("Content-Transfer-Encoding = %q, want base64", cte)
	}
	dec, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(string(a.body)), ""))
	if err != nil {
		t.Fatalf("decode base64 allegato 1: %v", err)
	}
	if string(dec) != "contenuto allegato" {
		t.Errorf("allegato 1 decodificato = %q", dec)
	}

	// Terza parte: file senza estensione → application/octet-stream.
	b := parts[2]
	if ct := b.header.Get("Content-Type"); ct != "application/octet-stream" {
		t.Errorf("allegato senza estensione Content-Type = %q, want application/octet-stream", ct)
	}
	if b.header.Get("Content-Disposition") != `attachment; filename="blob"` {
		t.Errorf("Content-Disposition = %q", b.header.Get("Content-Disposition"))
	}
	dec, err = base64.StdEncoding.DecodeString(strings.Join(strings.Fields(string(b.body)), ""))
	if err != nil {
		t.Fatalf("decode base64 allegato 2: %v", err)
	}
	if !bytes.Equal(dec, bin) {
		t.Errorf("allegato 2 decodificato = %q, want %q", dec, bin)
	}

	// Base64 a righe ≤ 76 caratteri con CRLF.
	if strings.Contains(s, strings.Repeat("A", 77)+"") {
		t.Error("base64 oltre 76 colonne")
	}
	if regexp.MustCompile(`(?m)^[A-Za-z0-9+/=]{77,}\r?$`).FindString(s) != "" {
		t.Error("trovata linea base64 > 76 caratteri")
	}
}

// TestBuildMIMEMissingFile: allegato inesistente → errore wrapped.
func TestBuildMIMEMissingFile(t *testing.T) {
	m := msg.Outgoing{
		From:        "me@example.com",
		To:          []string{"a@b.com"},
		Subject:     "x",
		BodyText:    "y",
		Attachments: []string{filepath.Join(t.TempDir(), "inesistente.txt")},
	}
	out, err := buildMIME(m, time.Now())
	if err == nil {
		t.Fatalf("want errore per file inesistente, output:\n%s", out)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("errore non wrappa os.ErrNotExist: %v", err)
	}
	if !strings.Contains(err.Error(), "inesistente.txt") {
		t.Errorf("errore non contiene il path: %v", err)
	}
}

// mimePart è una parte MIME decodificata (media type, parametri, header, corpo).
type mimePart struct {
	mediaType string
	params    map[string]string
	header    textproto.MIMEHeader
	body      []byte
}

// readParts scompone un corpo multipart (boundary dato) nelle sue parti.
func readParts(t *testing.T, r io.Reader, boundary string) []mimePart {
	t.Helper()
	mr := multipart.NewReader(r, boundary)
	var parts []mimePart
	for {
		p, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			return parts
		}
		if err != nil {
			t.Fatalf("NextPart: %v", err)
		}
		body, err := io.ReadAll(p)
		if err != nil {
			t.Fatalf("read part: %v", err)
		}
		mt, params, err := mime.ParseMediaType(p.Header.Get("Content-Type"))
		if err != nil {
			t.Fatalf("Content-Type parte %q: %v", p.Header.Get("Content-Type"), err)
		}
		parts = append(parts, mimePart{mediaType: mt, params: params, header: p.Header, body: body})
	}
}

// topLevelParts verifica il media type di primo livello del messaggio e
// ne ritorna le parti.
func topLevelParts(t *testing.T, out []byte, wantType string) []mimePart {
	t.Helper()
	m, err := mail.ReadMessage(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("mail.ReadMessage: %v", err)
	}
	mt, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("Content-Type top-level: %v", err)
	}
	if mt != wantType {
		t.Fatalf("Content-Type top-level = %q, want %q", mt, wantType)
	}
	return readParts(t, m.Body, params["boundary"])
}

// TestBuildMIMEMarkdownAlternative (Piano 7.2, Task 5): con Markdown e senza
// allegati → multipart/alternative; text/plain = markdown sorgente, text/html =
// goldmark con GFM (grassetto, tabella, citazione) e meta charset utf-8.
func TestBuildMIMEMarkdownAlternative(t *testing.T) {
	now := time.Date(2026, 11, 21, 10, 30, 0, 0, time.UTC)
	m := msg.Outgoing{
		From:     "me@example.com",
		To:       []string{"a@b.com"},
		Subject:  "Md",
		BodyText: "**forte** nota\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n> citata\n",
		Markdown: true,
	}
	out, err := buildMIME(m, now)
	if err != nil {
		t.Fatalf("buildMIME: %v", err)
	}
	parts := topLevelParts(t, out, "multipart/alternative")
	if len(parts) != 2 {
		t.Fatalf("parti = %d, want 2 (text/plain + text/html)", len(parts))
	}
	if parts[0].mediaType != "text/plain" || parts[0].params["charset"] != "utf-8" {
		t.Errorf("prima parte = %q %v, want text/plain charset=utf-8", parts[0].mediaType, parts[0].params)
	}
	if !bytes.Contains(parts[0].body, []byte("**forte** nota")) {
		t.Errorf("text/plain non contiene il markdown sorgente: %q", parts[0].body)
	}
	if parts[1].mediaType != "text/html" || parts[1].params["charset"] != "utf-8" {
		t.Errorf("seconda parte = %q %v, want text/html charset=utf-8", parts[1].mediaType, parts[1].params)
	}
	html := string(parts[1].body)
	for _, want := range []string{`<meta charset="utf-8">`, "<strong>forte</strong>", "<table>", "<blockquote>"} {
		if !strings.Contains(html, want) {
			t.Errorf("text/html manca %q in:\n%s", want, html)
		}
	}
}

// TestBuildMIMEMarkdownWithAttachments: con Markdown e allegati →
// multipart/mixed che contiene PRIMA il multipart/alternative, POI gli allegati.
func TestBuildMIMEMarkdownWithAttachments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(path, []byte("allegato"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := msg.Outgoing{
		From:        "me@example.com",
		To:          []string{"a@b.com"},
		Subject:     "Md allegati",
		BodyText:    "*corsivo*",
		Markdown:    true,
		Attachments: []string{path},
	}
	out, err := buildMIME(m, time.Now())
	if err != nil {
		t.Fatalf("buildMIME: %v", err)
	}
	top := topLevelParts(t, out, "multipart/mixed")
	if len(top) != 2 {
		t.Fatalf("parti top-level = %d, want 2 (alternative + allegato)", len(top))
	}
	if top[0].mediaType != "multipart/alternative" {
		t.Fatalf("prima parte = %q, want multipart/alternative", top[0].mediaType)
	}
	inner := readParts(t, bytes.NewReader(top[0].body), top[0].params["boundary"])
	if len(inner) != 2 || inner[0].mediaType != "text/plain" || inner[1].mediaType != "text/html" {
		t.Fatalf("alternative annidato = %d parti, want text/plain + text/html", len(inner))
	}
	if !strings.Contains(string(inner[1].body), "<em>corsivo</em>") {
		t.Errorf("text/html annidato senza <em>: %q", inner[1].body)
	}
	if cd := top[1].header.Get("Content-Disposition"); cd != `attachment; filename="notes.txt"` {
		t.Errorf("Content-Disposition allegato = %q", cd)
	}
	if cte := top[1].header.Get("Content-Transfer-Encoding"); cte != "base64" {
		t.Errorf("Content-Transfer-Encoding allegato = %q, want base64", cte)
	}
}

// TestBuildMIMEMarkdownEscapesUserScript: l'HTML scritto dall'utente NON esce
// come tag vivo (renderer senza html.WithUnsafe); nei blocchi di codice è
// escapato.
func TestBuildMIMEMarkdownEscapesUserScript(t *testing.T) {
	m := msg.Outgoing{
		From:     "me@example.com",
		To:       []string{"a@b.com"},
		Subject:  "xss",
		BodyText: "ciao\n\n<script>alert(1)</script>\n\n```\n<script>x</script>\n```\n",
		Markdown: true,
	}
	out, err := buildMIME(m, time.Now())
	if err != nil {
		t.Fatalf("buildMIME: %v", err)
	}
	html := string(topLevelParts(t, out, "multipart/alternative")[1].body)
	if strings.Contains(strings.ToLower(html), "<script") {
		t.Errorf("text/html contiene un <script> vivo:\n%s", html)
	}
	if !strings.Contains(html, "&lt;script&gt;x&lt;/script&gt;") {
		t.Errorf("codice non escapato nel blocco:\n%s", html)
	}
}

// Piano 7.2, fix review: il testo che sembra HTML ("<nome>", "<script>")
// nella parte text/html deve restare visibile come testo escapato, non
// sparire né diventare un tag vivo (goldmark senza WithUnsafe).
func TestBuildMIMEMarkdownRawHTMLAsEscapedText(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{"inline", "Ciao <nome>, come va?", "Ciao &lt;nome&gt;, come va?"},
		{"blocco", "<script>alert(1)</script>", "&lt;script&gt;alert(1)&lt;/script&gt;"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := msg.Outgoing{
				From:     "me@example.com",
				To:       []string{"a@b.com"},
				Subject:  "raw",
				BodyText: c.body,
				Markdown: true,
			}
			out, err := buildMIME(m, time.Now())
			if err != nil {
				t.Fatalf("buildMIME: %v", err)
			}
			html := string(topLevelParts(t, out, "multipart/alternative")[1].body)
			if !strings.Contains(html, c.want) {
				t.Errorf("text/html non contiene %q:\n%s", c.want, html)
			}
			if strings.Contains(html, "raw HTML omitted") {
				t.Errorf("text/html contiene il segnaposto goldmark:\n%s", html)
			}
		})
	}
}

// TestBuildMIMEMarkdownSignature: la firma appesa dal composer ("\n\n--\n" +
// firma) resta su righe separate nell'HTML (hard line break, niente unione in
// un unico paragrafo); il text/plain resta invariato.
func TestBuildMIMEMarkdownSignature(t *testing.T) {
	m := msg.Outgoing{
		From:     "me@example.com",
		To:       []string{"a@b.com"},
		Subject:  "firma",
		BodyText: "corpo\n\n--\nAndrea\nSignorile",
		Markdown: true,
	}
	out, err := buildMIME(m, time.Now())
	if err != nil {
		t.Fatalf("buildMIME: %v", err)
	}
	parts := topLevelParts(t, out, "multipart/alternative")
	html := string(parts[1].body)
	if !strings.Contains(html, "--<br>\r\nAndrea<br>\r\nSignorile") {
		t.Errorf("firma non su righe separate in:\n%s", html)
	}
	if !bytes.Contains(parts[0].body, []byte("--\r\nAndrea\r\nSignorile")) {
		t.Errorf("text/plain della firma alterato: %q", parts[0].body)
	}
}

// headerValue estrae il valore (compattato) di un header dalla riga grezza.
func headerValue(s, key string) string {
	for line := range strings.SplitSeq(s, "\r\n") {
		if k, v, ok := strings.Cut(line, ": "); ok && k == key {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
