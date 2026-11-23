package mail

// Test del Mock per mailbox (Piano 7, Task 1): ListMessages/Search filtrano per
// mailbox dell'ID, Trash sposta cambiando il prefisso, ListMailboxes e Err.

import (
	"context"
	"errors"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// errTest è l'errore iniettato nei Mock di questi test.
var errTest = errors.New("errore iniettato")

// TestMockListFiltersByMailbox: Label vuota = INBOX; Label "Work" vede solo i
// messaggi con ID "Work:...".
func TestMockListFiltersByMailbox(t *testing.T) {
	m := newMock() // 5 messaggi in INBOX
	m.Msgs = append(m.Msgs, &msg.Message{Ref: msg.MessageRef{ID: msg.NewID("Work", 9), Subject: "in work"}})
	ctx := context.Background()

	inbox, err := m.ListMessages(ctx, msg.Query{}, Page{})
	if err != nil || len(inbox) != 5 {
		t.Fatalf("INBOX = %d ref (err %v), voglio 5", len(inbox), err)
	}
	work, err := m.ListMessages(ctx, msg.Query{Label: "Work"}, Page{})
	if err != nil || len(work) != 1 || work[0].Subject != "in work" {
		t.Fatalf("Work = %+v (err %v), voglio solo [in work]", work, err)
	}
	found, err := m.Search(ctx, msg.Query{Label: "Work"})
	if err != nil || len(found) != 1 {
		t.Errorf("Search Work = %d ref (err %v), voglio 1", len(found), err)
	}
}

// TestMockListMailboxesDefault: senza Mailboxes configurate il Mock espone
// INBOX (inbox) e il cestino Gmail (trash); Err viene propagato.
func TestMockListMailboxesDefault(t *testing.T) {
	m := &Mock{}
	got, err := m.ListMailboxes(context.Background())
	if err != nil {
		t.Fatalf("ListMailboxes: %v", err)
	}
	want := []Mailbox{{Name: "INBOX", Role: "inbox"}, {Name: "[Gmail]/Drafts", Role: "drafts"}, {Name: "[Gmail]/Trash", Role: "trash"}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("ListMailboxes = %+v, voglio %+v", got, want)
	}

	m.Err = errTest
	if _, err := m.ListMailboxes(context.Background()); err == nil {
		t.Error("ListMailboxes con Err: err = nil, voglio errore")
	}
}

// TestMockTrashMovesMessage: il messaggio esce da INBOX ed entra nel cestino
// con un nuovo ID (prefisso del cestino).
func TestMockTrashMovesMessage(t *testing.T) {
	m := newMock()
	ctx := context.Background()
	if _, err := m.Trash(ctx, []msg.MessageID{msg.NewID("INBOX", 2)}); err != nil {
		t.Fatalf("Trash: %v", err)
	}
	inbox, _ := m.ListMessages(ctx, msg.Query{}, Page{})
	if len(inbox) != 4 {
		t.Errorf("INBOX dopo Trash = %d, voglio 4", len(inbox))
	}
	trash, _ := m.ListMessages(ctx, msg.Query{Label: "[Gmail]/Trash"}, Page{})
	if len(trash) != 1 || trash[0].Subject != "Secondo" {
		t.Fatalf("cestino = %+v, voglio [Secondo]", trash)
	}
	if mb, _, err := trash[0].ID.Split(); err != nil || mb != "[Gmail]/Trash" {
		t.Errorf("ID nel cestino = %q, voglio prefisso [Gmail]/Trash", trash[0].ID)
	}
}

// TestMockTrashErrors: nessun cestino, id già nel cestino, id inesistente e
// Err iniettato rifiutano senza azione parziale.
func TestMockTrashErrors(t *testing.T) {
	ctx := context.Background()

	noTrash := newMock()
	noTrash.Mailboxes = []Mailbox{{Name: "INBOX", Role: "inbox"}}
	if _, err := noTrash.Trash(ctx, []msg.MessageID{msg.NewID("INBOX", 1)}); err == nil {
		t.Error("Trash senza cestino: err = nil")
	}
	if got, _ := noTrash.ListMessages(ctx, msg.Query{}, Page{}); len(got) != 5 {
		t.Errorf("INBOX dopo Trash senza cestino = %d, voglio 5", len(got))
	}

	m := newMock()
	m.Msgs = append(m.Msgs, &msg.Message{Ref: msg.MessageRef{ID: msg.NewID("[Gmail]/Trash", 1), Subject: "già"}})
	if _, err := m.Trash(ctx, []msg.MessageID{msg.NewID("INBOX", 1), msg.NewID("[Gmail]/Trash", 1)}); err == nil {
		t.Error("Trash con id nel cestino: err = nil")
	}
	if got, _ := m.ListMessages(ctx, msg.Query{}, Page{}); len(got) != 5 {
		t.Errorf("INBOX dopo rifiuto = %d, voglio 5 (nessuna azione)", len(got))
	}
	if _, err := m.Trash(ctx, []msg.MessageID{msg.NewID("INBOX", 99)}); err == nil {
		t.Error("Trash di id inesistente: err = nil")
	}

	failing := newMock()
	failing.Err = errTest
	if _, err := failing.Trash(ctx, []msg.MessageID{msg.NewID("INBOX", 1)}); err == nil {
		t.Error("Trash con Err: err = nil")
	}
}
