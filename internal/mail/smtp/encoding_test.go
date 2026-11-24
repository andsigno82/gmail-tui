package smtp

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Piano 12a, Task 1 (D12-S2): le parti di testo dichiarano sempre come viaggiano.

var fissoCTE = time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)

// decodeQP decodifica un corpo quoted-printable col decoder della libreria standard.
func decodeQP(t *testing.T, r io.Reader) string {
	t.Helper()
	b, err := io.ReadAll(quotedprintable.NewReader(r))
	if err != nil {
		t.Fatalf("decode quoted-printable: %v", err)
	}
	return string(b)
}

// corpoQM legge un messaggio a una sola parte e ne decodifica il corpo.
func corpoQM(t *testing.T, out []byte) string {
	t.Helper()
	m, err := mail.ReadMessage(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	return decodeQP(t, m.Body)
}

// rigaLunga verifica che nessuna riga superi 998 byte (CRLF escluso).
func rigaLunga(t *testing.T, out []byte) {
	t.Helper()
	for i, riga := range strings.Split(string(out), "\n") {
		if len(strings.TrimSuffix(riga, "\r")) > 998 {
			t.Errorf("riga %d di %d byte, oltre il limite RFC 5322", i, len(riga))
		}
	}
}

// cteDichiarati ritorna i Content-Transfer-Encoding delle parti di primo livello,
// nell'ordine, come sono sul filo. topLevelParts legge con multipart.Reader.NextPart,
// che nasconde quell'intestazione sulle parti quoted-printable mentre ne decodifica
// il corpo: e proprio la dichiarazione da verificare, quindi qui la parte si legge
// con NextRawPart.
func cteDichiarati(t *testing.T, out []byte) []string {
	t.Helper()
	m, err := mail.ReadMessage(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("mail.ReadMessage: %v", err)
	}
	_, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("Content-Type top-level: %v", err)
	}
	mr := multipart.NewReader(m.Body, params["boundary"])
	var ctes []string
	for {
		p, err := mr.NextRawPart()
		if errors.Is(err, io.EOF) {
			return ctes
		}
		if err != nil {
			t.Fatalf("NextRawPart: %v", err)
		}
		ctes = append(ctes, p.Header.Get("Content-Transfer-Encoding"))
	}
}

// TestBuildMIMEAsciiStaysSevenBit: testo ASCII con righe corte → 7bit dichiarato
// e corpo identico al sorgente (con CRLF).
func TestBuildMIMEAsciiStaysSevenBit(t *testing.T) {
	m := msg.Outgoing{From: "me@example.com", To: []string{"a@b.com"}, Subject: "Salve", BodyText: "riga1\nriga2\n"}
	out, err := buildMIME(m, fissoCTE)
	if err != nil {
		t.Fatalf("buildMIME: %v", err)
	}
	want := "Content-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 7bit\r\n\r\nriga1\r\nriga2\r\n"
	if !strings.Contains(string(out), want) {
		t.Errorf("testo ASCII non esce 7bit com'e':\n%s", out)
	}
}

// TestBuildMIMEAccentsGoQuotedPrintable: accenti, spazi in fondo alla riga e un
// segno "=" → quoted-printable; il messaggio e tutto ASCII e decodifica al sorgente.
func TestBuildMIMEAccentsGoQuotedPrintable(t *testing.T) {
	body := "Città  \r\nperché x=y\r\npiù su\r\n"
	m := msg.Outgoing{From: "me@example.com", To: []string{"a@b.com"}, Subject: "Salve", BodyText: body}
	out, err := buildMIME(m, fissoCTE)
	if err != nil {
		t.Fatalf("buildMIME: %v", err)
	}
	if !strings.Contains(string(out), "Content-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n") {
		t.Errorf("accenti senza quoted-printable:\n%s", out)
	}
	if i := bytes.IndexFunc(out, func(r rune) bool { return r >= 0x80 }); i >= 0 {
		t.Errorf("byte non ASCII in posizione %d: %q", i, out[max(0, i-20):min(len(out), i+20)])
	}
	if got := corpoQM(t, out); got != body {
		t.Errorf("corpo decodificato = %q, voglio %q", got, body)
	}
}

// TestBuildMIMELongLineGoQuotedPrintable: una riga sopra i 998 byte →
// quoted-printable, nessuna riga oltre il limite, decodifica al sorgente.
func TestBuildMIMELongLineGoQuotedPrintable(t *testing.T) {
	body := strings.Repeat("a", 1500) + "\r\nfine\r\n"
	m := msg.Outgoing{From: "me@example.com", To: []string{"a@b.com"}, Subject: "Salve", BodyText: body}
	out, err := buildMIME(m, fissoCTE)
	if err != nil {
		t.Fatalf("buildMIME: %v", err)
	}
	if !strings.Contains(string(out), "Content-Transfer-Encoding: quoted-printable\r\n") {
		t.Errorf("riga lunga senza quoted-printable:\n%s", out)
	}
	rigaLunga(t, out)
	if got := corpoQM(t, out); got != body {
		t.Errorf("corpo decodificato di %d byte, voglio %d byte", len(got), len(body))
	}
}

// TestBuildMIMEMarkdownPartsChooseOwnEncoding: le due parti di alternative
// decidono ognuna per se. Una riga di 996 caratteri resta 7bit nel sorgente, ma
// nell'HTML diventa <p>…</p> (1003 byte): quoted-printable. multipart decodifica
// le parti in trasparenza.
func TestBuildMIMEMarkdownPartsChooseOwnEncoding(t *testing.T) {
	riga := strings.Repeat("a", 996)
	m := msg.Outgoing{
		From: "me@example.com", To: []string{"a@b.com"}, Subject: "Md",
		BodyText: riga + "\n", Markdown: true,
	}
	out, err := buildMIME(m, fissoCTE)
	if err != nil {
		t.Fatalf("buildMIME: %v", err)
	}
	parts := topLevelParts(t, out, "multipart/alternative")
	dichiarati := cteDichiarati(t, out)
	if cte := dichiarati[0]; cte != "7bit" {
		t.Errorf("text/plain Content-Transfer-Encoding = %q, voglio 7bit", cte)
	}
	if !bytes.Equal(parts[0].body, []byte(riga+"\r\n")) {
		t.Errorf("text/plain di %d byte, voglio il sorgente con CRLF", len(parts[0].body))
	}
	if cte := dichiarati[1]; cte != "quoted-printable" {
		t.Errorf("text/html Content-Transfer-Encoding = %q, voglio quoted-printable", cte)
	}
	if !strings.Contains(string(parts[1].body), "<p>"+riga+"</p>") {
		t.Errorf("text/html decodificato senza il paragrafo: %q", parts[1].body)
	}
}

// TestBuildMIMEAttachmentTextPartDeclaresEncoding: con gli allegati la parte di
// testo dentro multipart/mixed porta la sua codifica.
func TestBuildMIMEAttachmentTextPartDeclaresEncoding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(path, []byte("allegato"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := msg.Outgoing{
		From: "me@example.com", To: []string{"a@b.com"}, Subject: "Salve",
		BodyText: "città", Attachments: []string{path},
	}
	out, err := buildMIME(m, fissoCTE)
	if err != nil {
		t.Fatalf("buildMIME: %v", err)
	}
	parts := topLevelParts(t, out, "multipart/mixed")
	if cte := cteDichiarati(t, out)[0]; cte != "quoted-printable" {
		t.Errorf("parte di testo Content-Transfer-Encoding = %q, voglio quoted-printable", cte)
	}
	if !bytes.Equal(parts[0].body, []byte("città")) {
		t.Errorf("parte di testo = %q", parts[0].body)
	}
}

// TestBuildDraftRoundTripsQuotedPrintable: una bozza con accenti e una riga
// lunga si riapre con lo stesso testo (msg.Parse decodifica la parte).
func TestBuildDraftRoundTripsQuotedPrintable(t *testing.T) {
	lunga := strings.Repeat("b", 1200)
	out := msg.Outgoing{
		From: "me@example.dev", To: []string{"anna@example.dev"}, Subject: "Bozza",
		BodyText: "Città\r\n" + lunga + "\r\n",
	}
	raw, err := BuildDraft(out, fissoCTE)
	if err != nil {
		t.Fatalf("BuildDraft: %v", err)
	}
	if !strings.Contains(string(raw), "Content-Transfer-Encoding: quoted-printable\r\n") {
		t.Errorf("bozza senza quoted-printable:\n%s", raw)
	}
	rigaLunga(t, raw)
	parsed, err := msg.Parse(msg.NewID("[Gmail]/Drafts", 1), bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("msg.Parse: %v", err)
	}
	if !strings.Contains(parsed.BodyText, "Città") || !strings.Contains(parsed.BodyText, lunga) {
		t.Errorf("bozza riaperta senza il testo di prima: %q", parsed.BodyText)
	}
}

// TestNeedsEncoding: la soglia e i casi limite della scelta 7bit / quoted-printable.
func TestNeedsEncoding(t *testing.T) {
	corta := strings.Repeat("a", 998)
	cases := []struct {
		name, body string
		want       bool
	}{
		{"ascii corto", "ciao\r\nmondo\r\n", false},
		{"vuoto", "", false},
		{"accento", "città\r\n", true},
		{"nul", "a\x00b", true},
		{"riga di 998", corta + "\r\n", false},
		{"riga di 999", corta + "a\r\n", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := needsEncoding(c.body); got != c.want {
				t.Errorf("needsEncoding(%q) = %v, voglio %v", c.body, got, c.want)
			}
		})
	}
}
