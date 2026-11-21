// Mutazioni e feedback (Task 7): i tasti `a`/`s`/`v`/`l` agiscono sul ref
// corrente — nell'inbox la riga selezionata, nel thread il messaggio aperto.
// Ogni mutazione gira in una tea.Cmd (la rete non blocca mai), dà feedback in
// status bar via opDone e al risultato rilista la lista corrente IN SILENZIO
// (refreshRefs: refs e cursore aggiornati, status intatto — il feedback
// dell'op deve restare visibile finché il prossimo evento non lo sostituisce).
package app

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"gmail-tui/internal/mail"
	"gmail-tui/internal/msg"
	"gmail-tui/internal/ui/styles"
)

// labelPrompt è il prompt del mini-input delle label (render nella status
// bar); la sua lunghezza in colonne serve a dimensionare l'input.
const labelPrompt = "etichetta: "

// newLabelInput costruisce il textinput del mini-prompt label (stili condivisi
// con la ricerca via newPromptInput). Nasce NON focalizzato: il focus arriva
// quando `l` apre il prompt.
func newLabelInput(st styles.Styles) textinput.Model {
	ti := newPromptInput(labelPrompt, "nome label", st)
	ti.CharLimit = 64
	return ti
}

// currentRef restituisce il ref su cui operare: nel thread è il ref del
// messaggio aperto (la copia più aggiornata: stato al fetch + MarkRead + op
// ottimistiche), nell'inbox è il ref della riga sotto il cursore.
func (m Model) currentRef() (msg.MessageRef, bool) {
	if m.view == viewThread {
		if !m.thread.Loaded() {
			return msg.MessageRef{}, false // fetch in corso: nessun ref
		}
		return m.thread.Ref(), true
	}
	if m.view == viewInbox {
		return m.inbox.SelectedRef()
	}
	return msg.MessageRef{}, false
}

// runOp esegue il tasto-mutazione richiesto sul ref corrente. Le op NON sono
// bloccate con op store in flight (pending > 0): il gate del backend
// serializza le chiamate (una op alla volta) e il Mock è sincrono; un
// doppio-toggle sullo stesso ref fra pressione e refresh può correre —
// accettato in v1, il refresh che segue l'opDone corregge la copia in lista.
// Senza ref corrente (lista vuota, thread in caricamento) le op sono un
// no-op silenzioso.
func (m Model) runOp(k string) (Model, tea.Cmd) {
	ref, ok := m.currentRef()
	if !ok {
		return m, nil
	}
	switch k {
	case "a": // archivia: niente conferma (undo non in v1, feedback chiaro)
		m.pending++
		return m, tea.Batch(archive(m.store, ref.ID), m.spinner.Tick)
	case "s": // stella: toggle
		on := !ref.Starred
		updated := ref
		updated.Starred = on
		m.applyLocalRef(updated)
		m.pending++
		return m, tea.Batch(star(m.store, ref.ID, on), m.spinner.Tick)
	case "v": // lettura: toggle (unread → read, read → unread)
		read := ref.Unread
		updated := ref
		updated.Unread = !read
		m.applyLocalRef(updated)
		m.pending++
		return m, tea.Batch(markRead(m.store, ref.ID, read, true), m.spinner.Tick)
	case "l": // label: mini-prompt (enter applica, esc annulla)
		m.label.Reset()
		m.labelOpen = true
		return m, m.label.Focus()
	}
	return m, nil
}

// applyLocalRef propaga lo stato ottimistico del ref alla vista corrente: nel
// thread la copia locale si aggiorna SUBITO (l'header non rende i flag, ma i
// toggle consecutivi dell'utente leggono questo stato); la lista inbox si
// corregge col refresh che segue l'opDone, quindi non si tocca qui.
func (m *Model) applyLocalRef(updated msg.MessageRef) {
	if m.view != viewThread || !m.thread.Loaded() {
		return
	}
	if m.thread.Ref().ID != updated.ID {
		return
	}
	m.thread.SetRef(updated)
	m.pendingRef = updated
}

// confirmLabel chiude il prompt e applica la label digitata al ref corrente
// (on=true). Nome vuoto: nessuna op (come un annullamento). Il ref è letto al
// momento della conferma: col prompt aperto i tasti vanno all'input, il
// cursore non può muoversi fra apertura e conferma.
func (m Model) confirmLabel() (Model, tea.Cmd) {
	name := strings.TrimSpace(m.label.Value())
	m.labelOpen = false
	m.label.Blur()
	if name == "" {
		return m, nil
	}
	ref, ok := m.currentRef()
	if !ok {
		return m, nil
	}
	m.pending++
	return m, tea.Batch(applyLabel(m.store, ref.ID, name, true), m.spinner.Tick)
}

// cancelLabel chiude il prompt senza operare (esc).
func (m Model) cancelLabel() (Model, tea.Cmd) {
	m.labelOpen = false
	m.label.Blur()
	return m, nil
}

// markRead esegue MarkRead fuori dal loop UI. refresh=false per l'auto
// MarkRead al fetch (Task 5: solo feedback, la lista si aggiorna via
// Watch/nuova ricerca); refresh=true per il toggle `v` dell'utente. Il
// feedback dipende dalla direzione: "letta" / "segna non letta".
func markRead(st mail.MailStore, id msg.MessageID, read, refresh bool) tea.Cmd {
	label := "segna non letta"
	if read {
		label = "letta"
	}
	return func() tea.Msg {
		err := st.MarkRead(context.Background(), []msg.MessageID{id}, read)
		return opDone{label: label, err: err, refresh: refresh}
	}
}

// star esegue Star fuori dal loop UI: feedback "stella on"/"stella off".
func star(st mail.MailStore, id msg.MessageID, on bool) tea.Cmd {
	label := "stella off"
	if on {
		label = "stella on"
	}
	return func() tea.Msg {
		err := st.Star(context.Background(), []msg.MessageID{id}, on)
		return opDone{label: label, err: err, refresh: true}
	}
}

// archive esegue Archive fuori dal loop UI: feedback "archiviata". Nessuna
// conferma (undo non in v1, Piano 3 Task 7).
func archive(st mail.MailStore, id msg.MessageID) tea.Cmd {
	return func() tea.Msg {
		err := st.Archive(context.Background(), []msg.MessageID{id})
		return opDone{label: "archiviata", err: err, refresh: true}
	}
}

// applyLabel esegue ApplyLabel fuori dal loop UI: feedback "etichetta X
// applicata" (on) / "etichetta X rimossa" (off).
func applyLabel(st mail.MailStore, id msg.MessageID, label string, on bool) tea.Cmd {
	feedback := fmt.Sprintf("etichetta %s rimossa", label)
	if on {
		feedback = fmt.Sprintf("etichetta %s applicata", label)
	}
	return func() tea.Msg {
		err := st.ApplyLabel(context.Background(), []msg.MessageID{id}, label, on)
		return opDone{label: feedback, err: err, refresh: true}
	}
}

// requestRefresh prepara il re-list silenzioso della lista corrente (query
// corrente) — il meccanismo condiviso dal refresh dopo le op (Task 7) e dal
// feed Watch (Task 8). Coalescing: con un re-list già in flight NON si
// accatasta — marca dirty e il follow-up parte al risultato di quello in
// corso (msgsLoaded); i cambi arrivati durante il refresh non si perdono.
func (m *Model) requestRefresh() tea.Cmd {
	if m.refreshing {
		m.dirty = true
		return nil
	}
	m.dirty = false // il refresh che parte ora copre anche i cambi pendenti
	m.refreshing = true
	m.pending++
	return tea.Batch(refreshRefs(m.store, m.query, m.queryRaw), m.spinner.Tick)
}

// refreshRefs rilista la lista corrente fuori dal loop UI, nello stato
// silenzioso (msgsLoaded.silent): aggiorna refs e cursore senza toccare lo
// status. Riusa il pattern del load: query vuota → ListMessages (prima pagina
// da 50, come Init), query di ricerca → Search con la query corrente.
func refreshRefs(st mail.MailStore, q msg.Query, raw string) tea.Cmd {
	return func() tea.Msg {
		var refs []msg.MessageRef
		var err error
		if raw == "" {
			refs, err = st.ListMessages(context.Background(), msg.Query{}, mail.Page{Limit: 50})
		} else {
			refs, err = st.Search(context.Background(), q)
		}
		return msgsLoaded{refs: refs, err: err, query: raw, silent: true}
	}
}
