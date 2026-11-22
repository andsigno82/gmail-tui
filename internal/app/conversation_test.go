package app

import (
	"strings"
	"testing"
	"time"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Piano 8, Task 9: la lista mostra una riga per conversazione. Il Mock raggruppa
// per ThreadID: tre messaggi dello stesso thread sono una riga sola, con il
// messaggio più recente come ref di apertura e il contatore nel testo.
func TestListaUnaRigaPerConversazione(t *testing.T) {
	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	mk := func(uid uint32, from string, thread uint64, hours int, unread bool) *msg.Message {
		return &msg.Message{Ref: msg.MessageRef{
			ID: msg.NewID("INBOX", uid), From: from, Subject: "discussione",
			Date: base.Add(time.Duration(hours) * time.Hour), ThreadID: thread, Unread: unread,
		}}
	}
	msgs := []*msg.Message{
		mk(1, "anna@x.dev", 7, 1, false),
		mk(2, "mario@x.dev", 7, 2, false),
		mk(3, "anna@x.dev", 7, 3, true), // il più recente, non letto
		mk(4, "luca@x.dev", 0, 0, false),
	}
	m := pagedModel(t, &mail.Mock{Msgs: msgs})

	if len(m.refs) != 2 {
		t.Fatalf("righe = %d, voglio 2 (una conversazione di 3 messaggi + un singolo)", len(m.refs))
	}
	if m.refs[0].ID != msg.NewID("INBOX", 3) {
		t.Errorf("riga 0 ref = %s, voglio il messaggio più recente INBOX:3", m.refs[0].ID)
	}
	view := m.inbox.View()
	if !strings.Contains(view, "anna@x.dev e altri 1 (3)") {
		t.Errorf("lista = %q, voglio la riga della conversazione con partecipanti e contatore", view)
	}
	if !strings.Contains(view, "luca@x.dev") {
		t.Errorf("lista = %q, il messaggio singolo deve restare una riga", view)
	}
}

// Piano 8, Task 9: il riepilogo della conversazione sopravvive al merge di una
// pagina successiva e al refresh (la lista è refs più riepilogo per ID).
func TestRiepilogoConversazioneSopravvivePaginazione(t *testing.T) {
	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	c := mail.Conversation{
		Latest:       msg.MessageRef{ID: msg.NewID("INBOX", 9), From: "anna@x.dev", Subject: "thread", Date: base, ThreadID: 9},
		ThreadID:     9,
		Count:        4,
		Participants: []string{"anna@x.dev", "mario@x.dev"},
	}
	m := pagedModel(t, &mail.Mock{})
	m, _ = step(m, msgsLoaded{convs: []mail.Conversation{c}, refs: latestRefs([]mail.Conversation{c}), limit: pageSize, mailbox: "", gen: m.listGen})
	if !strings.Contains(m.inbox.View(), "anna@x.dev e altri 1 (4)") {
		t.Fatalf("lista = %q, voglio il riepilogo della conversazione", m.inbox.View())
	}
	// Una pagina successiva con un messaggio singolo: la conversazione resta com'è.
	single := mail.SingleConversation(msg.MessageRef{ID: msg.NewID("INBOX", 2), From: "luca@x.dev", Subject: "altro", Date: base.Add(-time.Hour)})
	m, _ = step(m, moreLoaded{convs: []mail.Conversation{single}, refs: latestRefs([]mail.Conversation{single}), mailbox: "", gen: m.listGen})
	if !strings.Contains(m.inbox.View(), "anna@x.dev e altri 1 (4)") {
		t.Errorf("dopo la pagina successiva la conversazione ha perso il riepilogo: %q", m.inbox.View())
	}
}
