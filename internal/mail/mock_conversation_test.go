package mail

// Piano 8, Task 5: conversazioni e ricerca Raw nel Mock (semantica Gmail:
// una riga per thread, Count/Unread/Participants su All Mail escluso il cestino).

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

func convMsg(box string, uid uint32, thread uint64, from string, date time.Time, unread bool) *msg.Message {
	return &msg.Message{Ref: msg.MessageRef{
		ID: msg.NewID(box, uid), From: from, Date: date, Unread: unread, ThreadID: thread,
	}}
}

func TestMockListConversationsOneRowPerThread(t *testing.T) {
	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	m := &Mock{Msgs: []*msg.Message{
		convMsg("INBOX", 1, 100, "Ann", base, false),
		convMsg("INBOX", 2, 100, "Bob", base.Add(2*time.Hour), true),
		convMsg("INBOX", 3, 200, "Cy", base.Add(time.Hour), false),
		convMsg("[Gmail]/Trash", 9, 100, "Dan", base.Add(3*time.Hour), true),
	}}
	convs, err := m.ListConversations(context.Background(), msg.Query{}, Page{})
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if len(convs) != 2 {
		t.Fatalf("len = %d, want 2 (una riga per thread)", len(convs))
	}
	c := convs[0]
	if c.ThreadID != 100 || c.Latest.ID != "INBOX:2" {
		t.Errorf("prima riga: thread %d latest %s, want 100 INBOX:2", c.ThreadID, c.Latest.ID)
	}
	if c.Count != 2 {
		t.Errorf("Count = %d, want 2 (il cestino non conta)", c.Count)
	}
	if strings.Join(c.Participants, ",") != "Ann,Bob" {
		t.Errorf("Participants = %v, want [Ann Bob]", c.Participants)
	}
	if !c.Unread {
		t.Error("Unread = false, want true (INBOX:2 non letto)")
	}
	if d := convs[1]; d.ThreadID != 200 || d.Count != 1 || d.Unread {
		t.Errorf("seconda riga: %+v", d)
	}
}

func TestMockConversationUnreadIgnoresTrash(t *testing.T) {
	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	m := &Mock{Msgs: []*msg.Message{
		convMsg("INBOX", 1, 100, "Ann", base, false),
		convMsg("INBOX", 2, 100, "Bob", base.Add(time.Hour), false),
		convMsg("[Gmail]/Trash", 9, 100, "Dan", base.Add(2*time.Hour), true),
	}}
	convs, err := m.ListConversations(context.Background(), msg.Query{}, Page{})
	if err != nil || len(convs) != 1 {
		t.Fatalf("convs = %v, err = %v", convs, err)
	}
	if convs[0].Unread {
		t.Error("Unread = true: un messaggio nel cestino non rende non letta la conversazione")
	}
}

func TestMockThreadMessagesChronologicalWithoutTrash(t *testing.T) {
	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	m := &Mock{Msgs: []*msg.Message{
		convMsg("INBOX", 2, 100, "Bob", base.Add(time.Hour), false),
		convMsg("INBOX", 1, 100, "Ann", base, false),
		convMsg("[Gmail]/Trash", 9, 100, "Dan", base.Add(2*time.Hour), true),
	}}
	refs, err := m.ThreadMessages(context.Background(), 100)
	if err != nil {
		t.Fatalf("ThreadMessages: %v", err)
	}
	got := []string{}
	for _, r := range refs {
		got = append(got, string(r.ID))
	}
	if strings.Join(got, ",") != "INBOX:1,INBOX:2" {
		t.Errorf("ThreadMessages = %v, want [INBOX:1 INBOX:2] (cronologico, senza cestino)", got)
	}
	if _, err := m.ThreadMessages(context.Background(), 0); err == nil {
		t.Error("thread 0: atteso errore")
	}
}

func TestMockRawSimulatedSubsetAndUnsupportedErrors(t *testing.T) {
	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	m := &Mock{Msgs: []*msg.Message{
		convMsg("INBOX", 1, 100, "Ann", base, false),
		convMsg("INBOX", 2, 100, "Bob", base.Add(time.Hour), true),
		convMsg("INBOX", 3, 200, "Cy", base, true),
	}}
	ctx := context.Background()
	convs, err := m.ListConversations(ctx, msg.Query{Raw: "from:bob is:unread"}, Page{})
	if err != nil {
		t.Fatalf("Raw sottoinsieme: %v", err)
	}
	if len(convs) != 1 || convs[0].ThreadID != 100 {
		t.Errorf("from:bob is:unread: %+v, want un solo thread 100", convs)
	}
	if _, err := m.ListConversations(ctx, msg.Query{Raw: "older_than:7d"}, Page{}); err == nil {
		t.Error("older_than non simulato nel Mock: atteso errore esplicito")
	}
}

// Piano 10, review punto 6: la copia in "Tutti i messaggi" di un messaggio che ha
// un'altra copia (stesso GmailID, fuori da All Mail) non conta nel thread: Gmail mostra
// il messaggio una volta. Senza altra copia (dopo un archivia) la copia di All Mail sì.
func TestMockThreadMessagesCountsAllMailCopyOnce(t *testing.T) {
	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	m := &Mock{
		Mailboxes: []Mailbox{
			{Name: "INBOX", Role: "inbox"},
			{Name: "[Gmail]/Tutti i messaggi", Role: "all"},
		},
		Msgs: []*msg.Message{
			{Ref: msg.MessageRef{ID: msg.NewID("INBOX", 1), GmailID: 1, ThreadID: 5, Date: base}},
			{Ref: msg.MessageRef{ID: msg.NewID("[Gmail]/Tutti i messaggi", 9), GmailID: 1, ThreadID: 5, Date: base}},
			{Ref: msg.MessageRef{ID: msg.NewID("[Gmail]/Tutti i messaggi", 8), GmailID: 2, ThreadID: 5, Date: base.Add(time.Hour)}},
		},
	}
	got, err := m.ThreadMessages(t.Context(), 5)
	if err != nil {
		t.Fatalf("ThreadMessages: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("messaggi del thread = %d (%v), voglio 2: la copia di All Mail con altra copia non conta", len(got), got)
	}
	for _, r := range got {
		if r.ID == msg.NewID("[Gmail]/Tutti i messaggi", 9) {
			t.Errorf("la copia di All Mail di un messaggio con altra copia compare nel thread: %v", got)
		}
	}
}
