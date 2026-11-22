package imap

// Test delle bozze IMAP (Piano 9, Task 3) su imapmemserver: APPEND in Bozze,
// rimozione per UID solo in Bozze, rifiuto degli ID fuori da Bozze.

import (
	"context"
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

const draftsName = "[Gmail]/Drafts"

// setupDrafts crea la cartella Bozze (ruolo \Drafts) e una mail in INBOX.
func setupDrafts(t *testing.T) (*testServer, *Store) {
	t.Helper()
	ts := startTestServerCaps(t, gmailCaps) // UIDPLUS: APPEND con UID e rimozione per UID
	ts.createSpecialMailbox(draftsName, imap.MailboxAttrDrafts)
	ts.seedMail("Alice <alice@example.org>", "in inbox", "corpo", false, false)
	return ts, connectStore(t, ts)
}

// draftSubjects elenca gli oggetti della cartella Bozze.
func draftSubjects(t *testing.T, s *Store) []string {
	t.Helper()
	refs, err := s.ListMessages(context.Background(), msg.Query{Label: draftsName}, mail.Page{})
	if err != nil {
		t.Fatalf("ListMessages Bozze: %v", err)
	}
	return subjects(refs)
}

// TestSaveDraftAppendsInDrafts: la bozza finisce in Bozze con un ID della cartella.
func TestSaveDraftAppendsInDrafts(t *testing.T) {
	_, s := setupDrafts(t)
	id, err := s.SaveDraft(context.Background(), rawMail("me@example.org", "bozza uno", "corpo"), nil)
	if err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	if !strings.HasPrefix(string(id), draftsName+":") {
		t.Errorf("ID = %q, voglio prefisso %q", id, draftsName+":")
	}
	if got := draftSubjects(t, s); len(got) != 1 || got[0] != "bozza uno" {
		t.Errorf("Bozze = %v, voglio [bozza uno]", got)
	}
}

// TestSaveDraftReplaceRemovesOnlyOldDraft: con replace la vecchia bozza esce da
// Bozze, la nuova resta e l'INBOX non cambia.
func TestSaveDraftReplaceRemovesOnlyOldDraft(t *testing.T) {
	_, s := setupDrafts(t)
	old, err := s.SaveDraft(context.Background(), rawMail("me@example.org", "vecchia", "c"), nil)
	if err != nil {
		t.Fatalf("SaveDraft vecchia: %v", err)
	}
	if _, err := s.SaveDraft(context.Background(), rawMail("me@example.org", "nuova", "c"), []msg.MessageID{old}); err != nil {
		t.Fatalf("SaveDraft nuova: %v", err)
	}
	if got := draftSubjects(t, s); len(got) != 1 || got[0] != "nuova" {
		t.Errorf("Bozze = %v, voglio solo [nuova]", got)
	}
	inbox, _ := s.ListMessages(context.Background(), msg.Query{}, mail.Page{})
	if len(inbox) != 1 {
		t.Errorf("INBOX = %d messaggi, voglio 1", len(inbox))
	}
}

// TestDraftOpsRefuseIDsOutsideDrafts: DeleteDraft e replace con un ID di INBOX
// falliscono senza toccare la mail in INBOX.
func TestDraftOpsRefuseIDsOutsideDrafts(t *testing.T) {
	_, s := setupDrafts(t)
	inbox := msg.NewID("INBOX", 1)
	if err := s.DeleteDraft(context.Background(), []msg.MessageID{inbox}); err == nil {
		t.Error("DeleteDraft su INBOX: err = nil, voglio errore")
	}
	if _, err := s.SaveDraft(context.Background(), rawMail("me@example.org", "x", "c"), []msg.MessageID{inbox}); err == nil {
		t.Error("SaveDraft con replace in INBOX: err = nil, voglio errore")
	}
	refs, _ := s.ListMessages(context.Background(), msg.Query{}, mail.Page{})
	if len(refs) != 1 {
		t.Errorf("INBOX = %d messaggi, voglio 1 (nessuna rimozione)", len(refs))
	}
}

// TestSaveDraftNoDraftsMailbox: senza cartella con ruolo drafts il salvataggio
// fallisce con errore esplicito.
func TestSaveDraftNoDraftsMailbox(t *testing.T) {
	ts := startTestServerCaps(t, gmailCaps)
	ts.seedMail("Alice <alice@example.org>", "in inbox", "corpo", false, false)
	s := connectStore(t, ts)
	if _, err := s.SaveDraft(context.Background(), rawMail("me@example.org", "x", "c"), nil); err == nil {
		t.Error("SaveDraft senza Bozze: err = nil, voglio errore")
	}
}
