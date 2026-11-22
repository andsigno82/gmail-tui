package imap

// Piano 11, Task 2: Change.Refs porta le intestazioni dei nuovi messaggi INBOX
// (per la notifica desktop) senza segnarli come letti.

import (
	"strings"
	"testing"
	"time"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

func TestWatchNewMailCarriesHeadersWithoutSeen(t *testing.T) {
	ts := startTestServer(t)
	ts.seedMail("Base <base@example.org>", "Base", "corpo base", false, false)
	store := connectStore(t, ts)

	ch, err := store.Watch(t.Context())
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	settleIdle()

	uid := ts.seedMail("Ada <ada@example.org>", "Oggetto di prova", "corpo nuovo", false, false)

	c := recvChange(t, ch, 5*time.Second)
	if c.Kind != mail.ChangeNewMail {
		t.Fatalf("Kind = %d, voglio ChangeNewMail", c.Kind)
	}
	if len(c.Refs) != 1 {
		t.Fatalf("Refs = %d, voglio 1 (una per ID nuovo)", len(c.Refs))
	}
	r := c.Refs[0]
	if r.ID != msg.NewID("INBOX", uid) {
		t.Errorf("Refs[0].ID = %s, voglio %s", r.ID, msg.NewID("INBOX", uid))
	}
	if !strings.Contains(r.From, "ada@example.org") {
		t.Errorf("Refs[0].From = %q, voglio il mittente Ada", r.From)
	}
	if r.Subject != "Oggetto di prova" {
		t.Errorf("Refs[0].Subject = %q, voglio %q", r.Subject, "Oggetto di prova")
	}

	// Il fetch delle intestazioni usa ENVELOPE/FLAGS senza BODY: il messaggio
	// resta non letto (nessun \Seen aggiunto dal Watch).
	list, err := store.ListMessages(t.Context(), msg.Query{}, mail.Page{})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	for _, l := range list {
		if l.ID == r.ID && !l.Unread {
			t.Errorf("il messaggio nuovo risulta letto: il Watch non deve usare \\Seen")
		}
	}
}
