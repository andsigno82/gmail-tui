package mcpserver

import (
	"testing"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Piano 14, fix review 2: search_messages senza folder cerca nella cartella con
// ruolo "all" (ricerca come Gmail web) e dichiara la cartella usata.
func TestSearchEmptyFolderUsesAllMailAndDeclaresIt(t *testing.T) {
	const all = "[Gmail]/Tutti i messaggi"
	m1 := msgAt(1, "in tutti", "")
	m1.Ref.ID = msg.NewID(all, 1)
	m := &mail.Mock{
		Mailboxes: []mail.Mailbox{{Name: "INBOX", Role: "inbox"}, {Name: all, Role: "all"}},
		Msgs:      []*msg.Message{m1, msgAt(2, "solo inbox", "")},
		NoGmail:   true, // fixture senza X-GM-EXT-1: la ricerca Gmail (X-GM-RAW) non entra qui
	}
	cs := connect(t, m, Options{Version: "test"})

	_, data := unwrap(t, call(t, cs, "search_messages", map[string]any{"query": ""}))
	if data["folder"] != all {
		t.Fatalf("folder dichiarata = %v, attesa %q", data["folder"], all)
	}
	if items, _ := data["items"].([]any); len(items) != 1 {
		t.Fatalf("righe = %d, attesa 1 (solo la mailbox All)", len(items))
	}
}

// Piano 14, fix review 2: senza ruolo "all" la ricerca ricade su INBOX e lo dichiara.
func TestSearchEmptyFolderFallsBackToInbox(t *testing.T) {
	m := &mail.Mock{
		Mailboxes: []mail.Mailbox{{Name: "INBOX", Role: "inbox"}},
		Msgs:      []*msg.Message{msgAt(1, "x", "")},
		NoGmail:   true,
	}
	cs := connect(t, m, Options{Version: "test"})

	_, data := unwrap(t, call(t, cs, "search_messages", map[string]any{"query": ""}))
	if data["folder"] != "INBOX" {
		t.Fatalf("folder dichiarata = %v, atteso INBOX", data["folder"])
	}
}

// Piano 14, fix review 2: list_recent senza folder dichiara INBOX.
func TestListRecentDeclaresFolder(t *testing.T) {
	cs := connect(t, &mail.Mock{Msgs: []*msg.Message{msgAt(1, "x", "")}}, Options{Version: "test"})
	_, data := unwrap(t, call(t, cs, "list_recent", nil))
	if data["folder"] != "INBOX" {
		t.Fatalf("folder dichiarata = %v, atteso INBOX", data["folder"])
	}
}
