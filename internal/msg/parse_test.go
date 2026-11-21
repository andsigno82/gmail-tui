package msg

import (
	"strings"
	"testing"
	"time"
)

const multipartMail = `From: Alice Rossi <alice@example.com>
To: bob@example.com
Cc: carol@example.com
Subject: Contratto aggiornato
Date: Thu, 20 Nov 2026 10:32:00 +0100
Message-ID: <c1@example.com>
MIME-Version: 1.0
Content-Type: multipart/mixed; boundary="BOUND"

--BOUND
Content-Type: text/plain; charset=utf-8

Ciao Bob,
trovi il contratto in allegato.
--BOUND
Content-Type: application/pdf; name="contratto.pdf"
Content-Disposition: attachment; filename="contratto.pdf"

JVBERi0xLjQ=
--BOUND--
`

const htmlOnlyMail = `From: Newsletter <news@example.org>
To: bob@example.com
Subject: Ultimi articoli
Date: Thu, 19 Nov 2026 08:00:00 +0100
MIME-Version: 1.0
Content-Type: text/html; charset=utf-8

<html><body><h1>Titolo</h1><p>Leggi <a href="https://example.org">qui</a>.</p></body></html>
`

const multipartALTMail = `From: Promozioni <promo@example.net>
To: bob@example.com
Subject: Offerta di novembre
Date: Thu, 19 Nov 2026 09:00:00 +0100
MIME-Version: 1.0
Content-Type: multipart/alternative; boundary="ALT"

--ALT
Content-Type: text/plain; charset=utf-8

Sconto del 30%: visita https://example.net/sconto.
--ALT
Content-Type: text/html; charset=utf-8

<html><body><p>Sconto del 30%: <a href="https://example.net/sconto">clicca qui</a>.</p></body></html>
--ALT--
`

func TestParseMultipart(t *testing.T) {
	m, err := Parse(NewID("INBOX", 42), strings.NewReader(multipartMail))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if m.Ref.Subject != "Contratto aggiornato" {
		t.Errorf("Subject = %q", m.Ref.Subject)
	}
	if m.Ref.From != "Alice Rossi <alice@example.com>" {
		t.Errorf("From = %q", m.Ref.From)
	}
	want := time.Date(2026, 11, 20, 10, 32, 0, 0, time.FixedZone("", 3600))
	if !m.Ref.Date.Equal(want) {
		t.Errorf("Date = %v, atteso %v", m.Ref.Date, want)
	}
	if m.BodyText != "Ciao Bob,\ntrovi il contratto in allegato.\n" &&
		!strings.Contains(m.BodyText, "trovi il contratto") {
		t.Errorf("BodyText inatteso: %q", m.BodyText)
	}
	if len(m.Attachments) != 1 {
		t.Fatalf("allegati = %d, atteso 1", len(m.Attachments))
	}
	a := m.Attachments[0]
	if a.Filename != "contratto.pdf" || a.ContentType != "application/pdf" {
		t.Errorf("allegato = %+v", a)
	}
	if a.PartIndex != 0 || a.Size == 0 {
		t.Errorf("allegato indice/size = %d/%d", a.PartIndex, a.Size)
	}
}

// TestParseMultipartAlternative: multipart/alternative (text/plain + text/html)
// → il corpo resta quello testuale (text/plain vince) e HasHTML resta false
// (semantica invariata), MA BodyHTML porta l'HTML originale col link per
// "apri nel browser": la forma dominante della posta reale.
func TestParseMultipartAlternative(t *testing.T) {
	m, err := Parse(NewID("INBOX", 46), strings.NewReader(multipartALTMail))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !strings.Contains(m.BodyText, "Sconto del 30%") || strings.Contains(m.BodyText, "<a href") {
		t.Errorf("BodyText = %q, voglio la parte text/plain senza HTML", m.BodyText)
	}
	if m.HasHTML {
		t.Error("HasHTML = true, voglio false (c'è la parte testuale: semantica invariata)")
	}
	if !strings.Contains(m.BodyHTML, `href="https://example.net/sconto"`) {
		t.Errorf("BodyHTML = %q, voglio l'HTML originale col link", m.BodyHTML)
	}
}

func TestParseHTMLFallback(t *testing.T) {
	m, err := Parse(NewID("INBOX", 43), strings.NewReader(htmlOnlyMail))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !m.HasHTML {
		t.Error("HasHTML atteso true")
	}
	if !strings.Contains(m.BodyText, "Titolo") || !strings.Contains(m.BodyText, "https://example.org") {
		t.Errorf("BodyText HTML→testo inatteso: %q", m.BodyText)
	}
}

// TestParseBodyHTML: con solo-HTML il corpo HTML originale resta nel campo
// BodyHTML (serve alla TUI per "apri nel browser"); senza nessuna parte HTML
// resta vuoto. Con parte testuale MA parte HTML presente non è vuoto (vedi
// TestParseMultipartAlternative).
func TestParseBodyHTML(t *testing.T) {
	html, err := Parse(NewID("INBOX", 44), strings.NewReader(htmlOnlyMail))
	if err != nil {
		t.Fatalf("Parse html: %v", err)
	}
	if !strings.Contains(html.BodyHTML, "<a href=\"https://example.org\">") {
		t.Errorf("BodyHTML = %q, voglio l'HTML originale non convertito", html.BodyHTML)
	}
	txt, err := Parse(NewID("INBOX", 45), strings.NewReader(multipartMail))
	if err != nil {
		t.Fatalf("Parse text: %v", err)
	}
	if txt.BodyHTML != "" {
		t.Errorf("BodyHTML con text/plain = %q, voglio vuoto", txt.BodyHTML)
	}
}

func TestIDSplitRoundtrip(t *testing.T) {
	id := NewID("INBOX", 7)
	mb, uid, err := id.Split()
	if err != nil || mb != "INBOX" || uid != 7 {
		t.Fatalf("Split = %q %d %v", mb, uid, err)
	}
	if _, _, err := (MessageID("senza-duepunti")).Split(); err == nil {
		t.Error("ID malformato deve dare errore")
	}
}

func TestQueryIsEmpty(t *testing.T) {
	if !(Query{}).IsEmpty() {
		t.Error("Query zero-value deve essere empty")
	}
	if (Query{Unread: true}).IsEmpty() {
		t.Error("Query con filtri non deve essere empty")
	}
}
