// Contatori dei non letti del selettore cartelle (Piano 15, Task 15, D15-10,
// D15-T10): MailStore.UnreadCounts (STATUS UNSEEN, solo INBOX e label, Q15-4)
// gira in un tea.Cmd fuori dal loop UI. Si chiede all'apertura del selettore e,
// a selettore aperto, dopo un Change di posta nuova o di flag e dopo un'op che
// cambia lo stato di lettura o la composizione della lista; i contatori servono
// solo li, e all'apertura successiva si rileggono comunque. Ogni richiesta ha
// una generazione: il risultato di una richiesta superata e stantio e si scarta.
// Un errore non da toast: il selettore resta senza contatori.
package app

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
)

// unreadLoaded e il risultato di UnreadCounts per il selettore (Piano 15, Task 15).
type unreadLoaded struct {
	counts map[string]int
	err    error
	gen    int
}

// loadUnread esegue UnreadCounts fuori dal loop UI (risultato: unreadLoaded).
func loadUnread(st mail.MailStore, gen int) tea.Cmd {
	return func() tea.Msg {
		counts, err := st.UnreadCounts(context.Background())
		return unreadLoaded{counts: counts, err: err, gen: gen}
	}
}

// requestUnread apre una nuova generazione e lancia la lettura. Ricevitore
// puntatore come requestRefresh: va chiamato PRIMA di restituire il Model
// (gotcha del value receiver, AGENTS.md).
func (m *Model) requestUnread() tea.Cmd {
	m.unreadGen++
	return loadUnread(m.store, m.unreadGen)
}

// refreshUnreadIfOpen rilegge i contatori solo col selettore aperto.
func (m *Model) refreshUnreadIfOpen() tea.Cmd {
	if !m.folderOpen {
		return nil
	}
	return m.requestUnread()
}

// applyUnread consuma il risultato: una generazione diversa dall'ultima
// richiesta e stantia e si scarta; l'errore azzera i contatori (nessun toast).
func (m Model) applyUnread(ev unreadLoaded) Model {
	if ev.gen != m.unreadGen {
		return m
	}
	if ev.err != nil {
		m.folderPick.counts = nil
		return m
	}
	m.folderPick.counts = ev.counts
	return m
}
