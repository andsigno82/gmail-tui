package mail

// Test del Mock per le bozze (Piano 9, Task 2): SaveDraft appende nella cartella
// Bozze, replace rimuove solo le bozze vecchie, DeleteDraft e replace rifiutano
// ID fuori da Bozze senza toccare nulla.

import (
	"context"
	"strings"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

const testDraftsBox = "[Gmail]/Drafts"

// rawDraft è un RFC 5322 minimo per i test delle bozze.
func rawDraft(subject string) []byte {
	return []byte("From: a@x.dev\r\nTo: b@x.dev\r\nSubject: " + subject + "\r\n\r\ncorpo della bozza\r\n")
}

// TestMockSaveDraftAppearsInDrafts: la bozza ha un ID nella cartella Bozze ed
// è visibile in ListMessages con Label = Bozze.
func TestMockSaveDraftAppearsInDrafts(t *testing.T) {
	m := newMock()
	id, err := m.SaveDraft(context.Background(), rawDraft("prima"), nil)
	if err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	if !strings.HasPrefix(string(id), testDraftsBox+":") {
		t.Fatalf("ID = %q, voglio prefisso %q", id, testDraftsBox+":")
	}
	refs, err := m.ListMessages(context.Background(), msg.Query{Label: testDraftsBox}, Page{})
	if err != nil {
		t.Fatalf("ListMessages Bozze: %v", err)
	}
	if len(refs) != 1 || refs[0].ID != id || refs[0].Subject != "prima" {
		t.Errorf("Bozze = %+v, voglio una bozza con ID %q e oggetto \"prima\"", refs, id)
	}
}

// TestMockSaveDraftReplacesOld: con replace la bozza vecchia scompare, la nuova
// resta, l'INBOX non cambia.
func TestMockSaveDraftReplacesOld(t *testing.T) {
	m := newMock()
	old, err := m.SaveDraft(context.Background(), rawDraft("vecchia"), nil)
	if err != nil {
		t.Fatalf("SaveDraft vecchia: %v", err)
	}
	inbox := len(m.Msgs)
	nuova, err := m.SaveDraft(context.Background(), rawDraft("nuova"), []msg.MessageID{old})
	if err != nil {
		t.Fatalf("SaveDraft nuova: %v", err)
	}
	refs, _ := m.ListMessages(context.Background(), msg.Query{Label: testDraftsBox}, Page{})
	if len(refs) != 1 || refs[0].ID != nuova {
		t.Errorf("Bozze dopo replace = %+v, voglio solo %q", refs, nuova)
	}
	if got := len(m.Msgs); got != inbox {
		t.Errorf("messaggi = %d, voglio %d (replace non deve toccare altro)", got, inbox)
	}
}

// TestMockDraftOpsRefuseOtherMailboxes: replace o DeleteDraft con un ID fuori da
// Bozze falliscono senza modificare il Mock.
func TestMockDraftOpsRefuseOtherMailboxes(t *testing.T) {
	m := newMock()
	inbox := msg.NewID("INBOX", 1)
	before := len(m.Msgs)
	if _, err := m.SaveDraft(context.Background(), rawDraft("x"), []msg.MessageID{inbox}); err == nil {
		t.Error("SaveDraft con replace in INBOX: err = nil, voglio errore")
	}
	if err := m.DeleteDraft(context.Background(), []msg.MessageID{inbox}); err == nil {
		t.Error("DeleteDraft su INBOX: err = nil, voglio errore")
	}
	if got := len(m.Msgs); got != before {
		t.Errorf("messaggi = %d, voglio %d: l'INBOX non deve cambiare", got, before)
	}
}

// TestMockSaveDraftWithoutDraftsBox: senza mailbox con ruolo drafts il salvataggio
// fallisce.
func TestMockSaveDraftWithoutDraftsBox(t *testing.T) {
	m := newMock()
	m.Mailboxes = []Mailbox{{Name: "INBOX", Role: "inbox"}}
	if _, err := m.SaveDraft(context.Background(), rawDraft("x"), nil); err == nil {
		t.Error("SaveDraft senza Bozze: err = nil, voglio errore")
	}
}

// TestMockDeleteDraftRemovesFromDrafts: DeleteDraft toglie la bozza indicata.
func TestMockDeleteDraftRemovesFromDrafts(t *testing.T) {
	m := newMock()
	id, _ := m.SaveDraft(context.Background(), rawDraft("da scartare"), nil)
	if err := m.DeleteDraft(context.Background(), []msg.MessageID{id}); err != nil {
		t.Fatalf("DeleteDraft: %v", err)
	}
	refs, _ := m.ListMessages(context.Background(), msg.Query{Label: testDraftsBox}, Page{})
	if len(refs) != 0 {
		t.Errorf("Bozze = %+v, voglio vuota", refs)
	}
}
