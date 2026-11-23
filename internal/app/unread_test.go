package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/ui/toast"
)

// countStore avvolge un MailStore e controlla UnreadCounts (Piano 15, Task 15):
// conta le chiamate, restituisce counts oppure err; con seq il conteggio di INBOX
// è il numero progressivo della chiamata (per distinguere i risultati stantii).
type countStore struct {
	mail.MailStore
	mu     sync.Mutex
	counts map[string]int
	err    error
	seq    bool
	calls  int
}

func (s *countStore) UnreadCounts(context.Context) (map[string]int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	if s.seq {
		return map[string]int{"INBOX": s.calls}, nil
	}
	return s.counts, nil
}

func (s *countStore) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// applyExceptUnread applica i msg di una cmd salvo gli unreadLoaded (che il
// chiamante tiene da parte) e restituisce questi ultimi.
func applyExceptUnread(m Model, c tea.Cmd) (Model, []unreadLoaded) {
	var held []unreadLoaded
	for _, ms := range runCmd(c) {
		switch v := ms.(type) {
		case nil, spinner.TickMsg:
			continue
		case unreadLoaded:
			held = append(held, v)
			continue
		}
		m, _ = step(m, ms)
	}
	return m, held
}

func unreadModel(tb testing.TB, cs *countStore) Model {
	tb.Helper()
	cs.MailStore = folderMock(tb)
	return folderModel(tb, &recorder{MailStore: cs})
}

// TestUnreadCountsShownInPicker: i contatori arrivano nel selettore, solo per
// INBOX e label (Q15-4), e non per le speciali.
func TestUnreadCountsShownInPicker(t *testing.T) {
	cs := &countStore{counts: map[string]int{"INBOX": 3, "Work": 2, "[Gmail]/Sent": 5}}
	m := openPicker(t, unreadModel(t, cs))
	v := m.View().Content
	for _, want := range []string{"(3)", "(2)"} {
		if !strings.Contains(v, want) {
			t.Errorf("selettore senza il contatore %s: %q", want, v)
		}
	}
	if strings.Contains(v, "(5)") {
		t.Errorf("contatore sulle speciali (Q15-4): %q", v)
	}
}

// TestUnreadChangeReloads: un Change di posta nuova o di flag ricarica i
// contatori col selettore aperto.
func TestUnreadChangeReloads(t *testing.T) {
	for _, kind := range []mail.ChangeKind{mail.ChangeNewMail, mail.ChangeFlags} {
		cs := &countStore{seq: true}
		m := openPicker(t, unreadModel(t, cs))
		if !strings.Contains(m.View().Content, "(1)") {
			t.Fatalf("setup: contatore iniziale assente: %q", m.View().Content)
		}
		m, cmd := step(m, changeEvt{c: mail.Change{Kind: kind}})
		m.cancel() // il rearm di waitForChange risponde nil invece di bloccare
		m = applyAll(t, m, cmd)
		if got := cs.callCount(); got != 2 {
			t.Errorf("Change %v: UnreadCounts chiamato %d volte, voglio 2", kind, got)
		}
		if !strings.Contains(m.View().Content, "(2)") {
			t.Errorf("Change %v: contatori non ricaricati: %q", kind, m.View().Content)
		}
	}
}

// TestUnreadOpReloads: un'op che cambia lo stato di lettura (opDone con
// localRead) ricarica i contatori col selettore aperto.
func TestUnreadOpReloads(t *testing.T) {
	cs := &countStore{seq: true}
	m := openPicker(t, unreadModel(t, cs))
	m, cmd := step(m, opDone{label: "letta", localID: m.refs[0].ID, localRead: true})
	m = applyAll(t, m, cmd)
	if got := cs.callCount(); got != 2 {
		t.Errorf("opDone letta: UnreadCounts chiamato %d volte, voglio 2", got)
	}
	if !strings.Contains(m.View().Content, "(2)") {
		t.Errorf("contatori non ricaricati dopo l'op: %q", m.View().Content)
	}
}

// TestUnreadStaleResultDiscarded: il risultato di una richiesta superata da una
// più recente è scartato anche se arriva dopo.
func TestUnreadStaleResultDiscarded(t *testing.T) {
	cs := &countStore{seq: true}
	m := unreadModel(t, cs)
	m, cmd1 := step(m, pressKey("m"))
	m, old := applyExceptUnread(m, cmd1) // risultato 1 tenuto da parte
	m, cmd2 := step(m, changeEvt{c: mail.Change{Kind: mail.ChangeNewMail}})
	m.cancel()
	m, fresh := applyExceptUnread(m, cmd2)
	if len(old) != 1 || len(fresh) != 1 {
		t.Fatalf("risultati: old=%d fresh=%d, voglio 1 e 1", len(old), len(fresh))
	}
	m, _ = step(m, fresh[0])
	m, _ = step(m, old[0]) // arriva in ritardo
	v := m.View().Content
	if !strings.Contains(v, "(2)") || strings.Contains(v, "(1)") {
		t.Errorf("risultato stantio applicato: %q", v)
	}
}

// TestUnreadErrorLeavesNoCounts: errore di UnreadCounts → nessun contatore e
// nessun toast; l'albero resta usabile.
func TestUnreadErrorLeavesNoCounts(t *testing.T) {
	cs := &countStore{err: errors.New("boom")}
	m := openPicker(t, unreadModel(t, cs))
	if !m.folderOpen {
		t.Fatal("l'errore dei contatori ha chiuso il selettore")
	}
	if toastIs(m, toast.Error) || toastText(m) != "" {
		t.Errorf("toast per un errore dei contatori: %q", toastText(m))
	}
	if v := m.View().Content; strings.Contains(v, "(") || !strings.Contains(v, "Inviati") {
		t.Errorf("selettore con errore dei contatori: %q", v)
	}
}
