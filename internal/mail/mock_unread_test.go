package mail

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Piano 15, Task 2: contratti Mailbox.Delimiter e UnreadCounts.

func TestMailboxDelimiter(t *testing.T) {
	mb := Mailbox{Name: "Lavoro/Clienti", Delimiter: "/"}
	if mb.Delimiter != "/" {
		t.Fatalf("Delimiter = %q, voglio %q", mb.Delimiter, "/")
	}
}

// Piano 15, Task 17: senza messaggi la mappa ha tutte le cartelle di default a
// zero (prima era una mappa vuota: lo stub del Task 2). Non nil.
func TestMockUnreadCountsZeroSenzaMessaggi(t *testing.T) {
	var s MailStore = &Mock{}
	got, err := s.UnreadCounts(context.Background())
	if err != nil {
		t.Fatalf("UnreadCounts: %v", err)
	}
	if got == nil {
		t.Fatal("UnreadCounts = nil, voglio mappa non nil")
	}
	for _, b := range defaultMailboxes {
		if n, ok := got[b.Name]; !ok || n != 0 {
			t.Errorf("UnreadCounts[%q] = %d (presente %v), voglio 0", b.Name, n, ok)
		}
	}
}

func TestMockUnreadCountsErr(t *testing.T) {
	boom := errors.New("boom")
	m := &Mock{Err: boom}
	if _, err := m.UnreadCounts(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("err = %v, voglio %v", err, boom)
	}
}

// Piano 15, Task 17: UnreadCounts del Mock conta i non letti per cartella
// (chiave = Name), coerente con ListMessages(Unread, Label).
func TestMockUnreadCountsPerCartella(t *testing.T) {
	m := &Mock{
		Mailboxes: []Mailbox{
			{Name: "INBOX", Role: "inbox"},
			{Name: "Work"},
			{Name: "Lavoro/Clienti", Delimiter: "/"},
		},
		Msgs: []*msg.Message{
			{Ref: msg.MessageRef{ID: msg.NewID("INBOX", 1), Unread: true}},
			{Ref: msg.MessageRef{ID: msg.NewID("INBOX", 2)}},
			{Ref: msg.MessageRef{ID: msg.NewID("INBOX", 3), Unread: true}},
			{Ref: msg.MessageRef{ID: msg.NewID("Work", 1), Unread: true}},
			{Ref: msg.MessageRef{ID: msg.NewID("Lavoro/Clienti", 1)}},
		},
	}
	got, err := m.UnreadCounts(context.Background())
	if err != nil {
		t.Fatalf("UnreadCounts: %v", err)
	}
	want := map[string]int{"INBOX": 2, "Work": 1, "Lavoro/Clienti": 0}
	for box, n := range want {
		if got[box] != n {
			t.Errorf("UnreadCounts[%q] = %d, voglio %d (mappa %v)", box, got[box], n, got)
		}
	}
}

// Piano 15, Task 17: ogni contatore coincide con i messaggi non letti che
// ListMessages restituisce per la stessa cartella (nessuna divergenza fra
// badge e lista).
func TestMockUnreadCountsCoerenteConListMessages(t *testing.T) {
	m := &Mock{
		Mailboxes: []Mailbox{{Name: "INBOX", Role: "inbox"}, {Name: "Work"}},
		Msgs: []*msg.Message{
			{Ref: msg.MessageRef{ID: msg.NewID("INBOX", 1), Unread: true, Date: time.Unix(3, 0)}},
			{Ref: msg.MessageRef{ID: msg.NewID("INBOX", 2), Unread: true, Date: time.Unix(2, 0)}},
			{Ref: msg.MessageRef{ID: msg.NewID("Work", 1), Unread: true, Date: time.Unix(1, 0)}},
		},
	}
	got, err := m.UnreadCounts(context.Background())
	if err != nil {
		t.Fatalf("UnreadCounts: %v", err)
	}
	for _, b := range m.Mailboxes {
		refs, err := m.ListMessages(context.Background(), msg.Query{Label: b.Name, Unread: true}, Page{Limit: 1000})
		if err != nil {
			t.Fatalf("ListMessages %s: %v", b.Name, err)
		}
		if got[b.Name] != len(refs) {
			t.Errorf("%s: UnreadCounts = %d, ListMessages(Unread) = %d", b.Name, got[b.Name], len(refs))
		}
	}
}
