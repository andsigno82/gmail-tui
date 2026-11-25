package imap

// Piano 12a, Task 2: un messaggio che il FETCH non restituisce (espunto fra
// SEARCH e FETCH) non diventa una riga vuota.

import (
	"context"
	"strings"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Percorso Gmail: una riga per conversazione.
func TestListConversationsSkipsMessageMissingFromFetch(t *testing.T) {
	inbox := []gmailFetch{
		{uid: 1, thread: 100, login: "ann", date: "01-Nov-2026"},
		{uid: 2, thread: 200, login: "bob", date: "02-Nov-2026"}, // sparisce prima del FETCH
		{uid: 3, thread: 300, login: "cy", date: "03-Nov-2026"},
	}
	boxes := map[string][]gmailFetch{"INBOX": inbox, "[Gmail]/All Mail": inbox}
	addr := scriptedGmailServerOpts(t, boxes, gmailServerOpts{vanished: map[string]bool{"INBOX:2": true}})
	s := NewStore(Options{Addr: addr, Username: "u", Password: "p", Insecure: true})
	ctx := context.Background()
	if err := s.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = s.Close() }()

	convs, err := s.ListConversations(ctx, msg.Query{}, mail.Page{})
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if len(convs) != 2 {
		t.Fatalf("conversazioni = %d, voglio 2 (uid 3 e 1): %+v", len(convs), convs)
	}
	for i, want := range []string{"INBOX:3", "INBOX:1"} {
		if c := convs[i]; string(c.Latest.ID) != want || c.Latest.Subject == "" || c.Latest.From == "" {
			t.Errorf("conversazione %d = %+v, voglio %q con oggetto e mittente", i, c.Latest, want)
		}
	}
}

// Percorso senza Gmail (ListMessages e Search).
func TestListMessagesSkipsMessageMissingFromFetch(t *testing.T) {
	addr := scriptedIMAP(t, "IMAP4rev1", func(cmd string) []string {
		switch {
		case strings.HasPrefix(cmd, "SELECT"):
			return []string{"* 3 EXISTS"}
		case strings.HasPrefix(cmd, "UID SEARCH"):
			return []string{"* SEARCH 1 2 3"}
		case strings.HasPrefix(cmd, "UID FETCH"):
			return []string{
				`* 1 FETCH (UID 1 ENVELOPE (NIL "uno" (("Ann" NIL "ann" "x.test")) NIL NIL NIL NIL NIL NIL "<1@x>") FLAGS () INTERNALDATE "01-Nov-2026 10:00:00 +0000")`,
				`* 3 FETCH (UID 3 ENVELOPE (NIL "tre" (("Cy" NIL "cy" "x.test")) NIL NIL NIL NIL NIL NIL "<3@x>") FLAGS () INTERNALDATE "03-Nov-2026 10:00:00 +0000")`,
			}
		}
		return nil
	})
	s := NewStore(Options{Addr: addr, Username: "u", Password: "p", Insecure: true})
	ctx := context.Background()
	if err := s.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = s.Close() }()

	refs, err := s.ListMessages(ctx, msg.Query{}, mail.Page{})
	if err != nil || len(refs) != 2 {
		t.Fatalf("ListMessages: %v, %d refs, voglio 2", err, len(refs))
	}
	if refs[0].Subject != "tre" || refs[1].Subject != "uno" {
		t.Errorf("oggetti = %q, %q, voglio tre, uno", refs[0].Subject, refs[1].Subject)
	}
}
