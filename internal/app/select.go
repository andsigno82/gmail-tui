// Selezione multipla nella lista (Piano 10, Task 2, D10-S1/S3): `x` marca e smarca
// la conversazione sotto il cursore, `X` seleziona tutte le righe caricate (o le
// azzera se sono già tutte selezionate), `esc` azzera. La selezione è indicizzata
// per ID della riga (Conversation.Latest.ID, D10-T1): gli indici cambiano a ogni
// refresh, l'ID no. Si azzera al cambio di cartella/ricerca e dopo un'azione.
package app

import (
	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// setSel imposta la selezione (nil = nessuna) e la propaga alla lista, che disegna
// la casella ☑/☐. Il puntatore: chi chiama ha un Model locale da restituire.
func (m *Model) setSel(sel map[msg.MessageID]bool) {
	if len(sel) == 0 {
		sel = nil
	}
	m.sel = sel
	m.inbox.SetSelection(sel)
}

// toggleSel marca o smarca la riga sotto il cursore (`x`). Copia la mappa: il
// Model è un valore e le copie precedenti non devono cambiare.
func (m Model) toggleSel() (Model, tea.Cmd) {
	ref, ok := m.inbox.SelectedRef()
	if !ok {
		return m, nil
	}
	next := make(map[msg.MessageID]bool, len(m.sel)+1)
	for id := range m.sel {
		next[id] = true
	}
	if next[ref.ID] {
		delete(next, ref.ID)
	} else {
		next[ref.ID] = true
	}
	m.setSel(next)
	return m, nil
}

// selectAllRows seleziona tutte le righe caricate (`X`); se lo sono già tutte, le
// azzera. Lista vuota: nessun effetto.
func (m Model) selectAllRows() (Model, tea.Cmd) {
	convs := m.inbox.Conversations()
	if len(convs) == 0 {
		return m, nil
	}
	all := true
	for _, c := range convs {
		if !m.sel[c.Latest.ID] {
			all = false
			break
		}
	}
	if all {
		m.setSel(nil)
		return m, nil
	}
	next := make(map[msg.MessageID]bool, len(convs))
	for _, c := range convs {
		next[c.Latest.ID] = true
	}
	m.setSel(next)
	return m, nil
}

// pruneSel toglie dalla selezione le righe che non sono più nella lista (Piano 10,
// Task 2, D10-T1): un refresh o un'op può farle sparire, e il conteggio "N
// selezionate" non deve restare stantio. Confronto per ID della riga (m.refs).
func (m *Model) pruneSel() {
	if len(m.sel) == 0 {
		return
	}
	present := make(map[msg.MessageID]bool, len(m.refs))
	for _, r := range m.refs {
		present[r.ID] = true
	}
	next := make(map[msg.MessageID]bool, len(m.sel))
	for id := range m.sel {
		if present[id] {
			next[id] = true
		}
	}
	if len(next) != len(m.sel) {
		m.setSel(next)
	}
}
