package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/config"
	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/inbox"
)

// convViewFixture: una conversazione (thread 7) di quattro messaggi in ordine
// cronologico: anna (letto), mario (NON letto), tu in Inviati (letto), anna
// (letto, il più recente, in INBOX). La riga di lista è l'ultimo messaggio.
func convViewFixture(t *testing.T) *mail.Mock {
	t.Helper()
	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	mk := func(box string, uid uint32, from string, hours int, unread bool) *msg.Message {
		return &msg.Message{
			Ref: msg.MessageRef{
				ID: msg.NewID(box, uid), From: from, Subject: "piano settimana",
				Date: base.Add(time.Duration(hours) * time.Hour), ThreadID: 7, Unread: unread,
			},
			BodyText: "corpo di " + from,
		}
	}
	return &mail.Mock{Msgs: []*msg.Message{
		mk("INBOX", 1, "anna@x.dev", 0, false),
		mk("INBOX", 2, "mario@x.dev", 1, true),
		mk("[Gmail]/Sent", 3, "tu@x.dev", 2, false),
		mk("INBOX", 4, "anna@x.dev", 3, false),
	}}
}

// unreadInFixture conta i messaggi del Mock ancora non letti.
func unreadInFixture(store *mail.Mock) int {
	n := 0
	for _, mm := range store.Msgs {
		if mm.Ref.Unread {
			n++
		}
	}
	return n
}

// Piano 8, Task 10: aprire una conversazione mostra tutti i suoi messaggi (in
// Inviati incluso) con i letti compressi, e segna letti i non letti.
func TestAperturaConversazioneMostraTuttiEMarcaLetti(t *testing.T) {
	store := convViewFixture(t)
	m := pagedModel(t, store)
	row := m.refs[0]

	m, cmd := step(m, inbox.SelectedMsg{Ref: row})
	var follow []tea.Cmd
	for _, ms := range runCmd(cmd) {
		var c tea.Cmd
		m, c = step(m, ms)
		follow = append(follow, c)
	}
	view := m.thread.View()
	if !strings.Contains(view, "▸ tu@x.dev") {
		t.Errorf("la risposta inviata deve comparire compressa nella conversazione:\n%s", view)
	}
	for _, c := range follow {
		runCmd(c)
	}
	if n := unreadInFixture(store); n != 0 {
		t.Errorf("aprire la conversazione deve segnare letti tutti i non letti: ne restano %d", n)
	}
}

// Piano 8, Task 10: nell'anteprima a due colonne il segna-come-letto dopo 2 s
// vale per tutti i non letti della conversazione, non solo per il più recente.
func TestAnteprimaConversazioneSegnaLettiTuttiDopoDueSecondi(t *testing.T) {
	store := convViewFixture(t)
	m := pagedModel(t, store)
	m.SetPrefs(config.LayoutSplit, config.PreviewMarkReadDelay)
	m, _ = step(m, tea.WindowSizeMsg{Width: 140, Height: 30})
	m = feedPreview(t, m)
	if !m.previewReady() {
		t.Fatalf("anteprima non pronta")
	}
	m, cmd := step(m, previewReadDue{tok: m.prevTok, id: m.prevRef.ID})
	runCmd(cmd)
	if n := unreadInFixture(store); n != 0 {
		t.Errorf("dopo 2 s la conversazione in anteprima deve essere tutta letta: ne restano %d", n)
	}
}
