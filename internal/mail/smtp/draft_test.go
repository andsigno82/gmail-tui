package smtp

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Piano 9, Task 4: BuildDraft produce il MIME della bozza. A differenza
// dell'invio conserva l'intestazione Bcc (la bozza è privata dell'utente, nella
// cartella Bozze); l'invio (buildMIME) non la scrive mai.

// TestBuildDraftRoundTripsWithMsgParse: To, Subject e corpo markdown sorgente
// tornano con msg.Parse (la ripresa parte da qui).
func TestBuildDraftRoundTripsWithMsgParse(t *testing.T) {
	out := msg.Outgoing{
		From:     "me@example.com",
		To:       []string{"anna@example.dev"},
		Subject:  "Bozza di prova",
		BodyText: "# Titolo\n\nciao **mondo**",
		Markdown: true,
	}
	raw, err := BuildDraft(out, time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("BuildDraft: %v", err)
	}
	parsed, err := msg.Parse(msg.NewID("[Gmail]/Drafts", 1), bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("msg.Parse: %v", err)
	}
	if got := strings.Join(parsed.To, ","); got != "anna@example.dev" {
		t.Errorf("To = %q, voglio anna@example.dev", got)
	}
	if parsed.Ref.Subject != "Bozza di prova" {
		t.Errorf("Subject = %q", parsed.Ref.Subject)
	}
	if !strings.Contains(parsed.BodyText, "ciao **mondo**") {
		t.Errorf("BodyText = %q, voglio il markdown sorgente", parsed.BodyText)
	}
}

// TestBuildDraftKeepsBccHeader: la bozza conserva Bcc nell'intestazione.
func TestBuildDraftKeepsBccHeader(t *testing.T) {
	out := msg.Outgoing{
		From: "me@example.com", To: []string{"a@x.dev"}, Bcc: []string{"segreto@x.dev"},
		Subject: "s", BodyText: "c",
	}
	raw, err := BuildDraft(out, time.Now())
	if err != nil {
		t.Fatalf("BuildDraft: %v", err)
	}
	if !strings.Contains(string(raw), "Bcc: segreto@x.dev\r\n") {
		t.Errorf("bozza senza intestazione Bcc:\n%s", raw)
	}
	parsed, err := msg.Parse(msg.NewID("[Gmail]/Drafts", 1), bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("msg.Parse: %v", err)
	}
	if got := strings.Join(parsed.Bcc, ","); got != "segreto@x.dev" {
		t.Errorf("Bcc dopo Parse = %q, voglio segreto@x.dev", got)
	}
}

// TestSendMIMEHasNoBccHeader: l'invio resta senza Bcc negli header (D9-1).
func TestSendMIMEHasNoBccHeader(t *testing.T) {
	out := msg.Outgoing{
		From: "me@example.com", To: []string{"a@x.dev"}, Bcc: []string{"segreto@x.dev"},
		Subject: "s", BodyText: "c",
	}
	raw, err := buildMIME(out, time.Now())
	if err != nil {
		t.Fatalf("buildMIME: %v", err)
	}
	if strings.Contains(string(raw), "Bcc") || strings.Contains(string(raw), "segreto") {
		t.Errorf("MIME di invio con Bcc negli header:\n%s", raw)
	}
}
