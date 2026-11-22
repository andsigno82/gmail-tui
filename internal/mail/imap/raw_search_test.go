package imap

// Piano 8, Task 8: ricerca X-GM-RAW nativa su All Mail (D8-4) e thread/label
// nei riferimenti di lista. Server scriptato, nessuna rete.

import (
	"context"
	"strings"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

func TestSearchRawUsesAllMailAndXGMRAW(t *testing.T) {
	allMail := []gmailFetch{
		{uid: 1, thread: 200, login: "cy", date: "01-Nov-2026", read: true},
		{uid: 2, thread: 100, login: "ann", date: "02-Nov-2026", read: false},
		{uid: 3, thread: 100, login: "bob", date: "03-Nov-2026", read: true},
		{uid: 4, thread: 300, login: "dan", date: "04-Nov-2026", read: false},
	}
	addr := scriptedGmailServer(t, map[string][]gmailFetch{"INBOX": {}, "[Gmail]/All Mail": allMail})
	s := NewStore(Options{Addr: addr, Username: "u", Password: "p", Insecure: true})
	ctx := context.Background()
	if err := s.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = s.Close() }()

	refs, err := s.Search(ctx, msg.Query{Raw: "is:unread"})
	if err != nil {
		t.Fatalf("Search Raw: %v", err)
	}
	got := make([]string, 0, len(refs))
	for _, r := range refs {
		got = append(got, string(r.ID))
	}
	// il server finto applica is:unread solo se X-GM-RAW arriva verbatim su All Mail
	want := "[Gmail]/All Mail:4,[Gmail]/All Mail:2"
	if strings.Join(got, ",") != want {
		t.Errorf("Search Raw = %v, want %s", got, want)
	}
}

func TestSearchRawWithoutGmailIsError(t *testing.T) {
	addr := scriptedIMAP(t, "IMAP4rev1 UIDPLUS", func(string) []string { return nil })
	s := NewStore(Options{Addr: addr, Username: "u", Password: "p", Insecure: true})
	ctx := context.Background()
	if err := s.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.Search(ctx, msg.Query{Raw: "older_than:7d"}); err == nil {
		t.Error("Raw senza Gmail: atteso errore esplicito, nessuno")
	}
}

func TestListMessagesCarriesGmailThreadID(t *testing.T) {
	inbox := []gmailFetch{
		{uid: 1, thread: 200, login: "cy", date: "01-Nov-2026", read: true},
		{uid: 2, thread: 100, login: "ann", date: "02-Nov-2026", read: true},
	}
	addr := scriptedGmailServer(t, map[string][]gmailFetch{"INBOX": inbox, "[Gmail]/All Mail": inbox})
	s := NewStore(Options{Addr: addr, Username: "u", Password: "p", Insecure: true})
	ctx := context.Background()
	if err := s.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = s.Close() }()

	refs, err := s.ListMessages(ctx, msg.Query{}, mail.Page{})
	if err != nil || len(refs) != 2 {
		t.Fatalf("ListMessages: %v, %d refs", err, len(refs))
	}
	if refs[0].ThreadID != 100 || refs[1].ThreadID != 200 {
		t.Errorf("ThreadID = %d,%d, want 100,200", refs[0].ThreadID, refs[1].ThreadID)
	}
	if refs[0].GmailID != 1002 {
		t.Errorf("GmailID = %d, want 1002", refs[0].GmailID)
	}
}
