package imap

// Test del fix Watch (Piano 7, Task 1): un'op su un'altra cartella lascia quella
// mailbox selezionata sulla connessione; l'IDLE successivo deve comunque
// osservare INBOX, altrimenti la posta nuova in INBOX non arriva.

import (
	"testing"
	"time"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// TestWatchIdlesOnInboxAfterOpOnOtherMailbox: Watch attivo → ListMessages su
// "Work" (lascia Work selezionata) → mail nuova in INBOX → entro 5s arriva
// ChangeNewMail con l'ID INBOX.
func TestWatchIdlesOnInboxAfterOpOnOtherMailbox(t *testing.T) {
	ts := startTestServer(t)
	ts.createMailbox("Work")
	ts.seedInMailbox("Work", "W <w@example.org>", "In work", "corpo", false, false)
	ts.seedMail("Base <base@example.org>", "Base", "corpo", false, false)
	store := connectStore(t, ts)

	ch, err := store.Watch(t.Context())
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	settleIdle()

	if _, err := store.ListMessages(t.Context(), msg.Query{Label: "Work"}, mail.Page{}); err != nil {
		t.Fatalf("ListMessages Work: %v", err)
	}
	settleIdle()

	uid := ts.seedMail("New <new@example.org>", "Watch me", "corpo nuovo", false, false)

	c := recvChange(t, ch, 5*time.Second)
	if c.Kind != mail.ChangeNewMail {
		t.Errorf("Kind = %d, voglio ChangeNewMail (%d)", c.Kind, mail.ChangeNewMail)
	}
	want := msg.NewID("INBOX", uid)
	if len(c.IDs) == 0 || c.IDs[len(c.IDs)-1] != want {
		t.Errorf("IDs = %v, voglio che termini con %s", c.IDs, want)
	}
}
