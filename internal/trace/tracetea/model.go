// Package tracetea avvolge il tea.Model radice misurando, per ogni messaggio, la
// durata di Update e della View che lo segue, con i byte della stringa resa
// (Piano 16, traccia dei tempi). PRIVACY: del messaggio si scrive solo il TIPO e,
// per i tasti, il nome di un tasto di comando; il testo digitato (qualunque
// tasto che produce caratteri) e sempre "char", cosi indirizzi, ricerche e corpi
// scritti nel composer non finiscono mai nel file.
package tracetea

import (
	"fmt"
	"strings"
	"sync"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/trace"
)

// screener e implementata dal modello radice dell'app: dice in quale vista siamo
// (stringa costante: inbox, thread, search, composer, wizard).
type screener interface{ TraceScreen() string }

// state e condiviso dalle copie del wrapper (il modello e passato per valore).
type state struct {
	mu      sync.Mutex
	pending *trace.Event // messaggio con Update gia misurato, in attesa della sua View

	firstView  bool // prima View emessa
	connected  bool // connResult visto
	listLoaded bool // primo msgsLoaded visto
	listView   bool // prima View dopo il caricamento della lista emessa
}

// Model e il wrapper: implementa tea.Model e inoltra ExitMessages.
type Model struct {
	inner tea.Model
	st    *state
}

// Wrap avvolge m; senza traccia installata ritorna m stesso (nessun costo).
func Wrap(m tea.Model) tea.Model {
	if !trace.Enabled() {
		return m
	}
	return Model{inner: m, st: &state{}}
}

// Init inoltra al modello interno.
func (w Model) Init() tea.Cmd { return w.inner.Init() }

// ExitMessages inoltra le note d'uscita dell'app (cfr. cmd/gmail-tui runProgram).
func (w Model) ExitMessages() []string {
	if em, ok := w.inner.(interface{ ExitMessages() []string }); ok {
		return em.ExitMessages()
	}
	return nil
}

// Update misura l'Update interno. L'evento "msg" si completa alla View successiva
// (flush) oppure, se un altro Update arriva prima, senza View.
func (w Model) Update(ev tea.Msg) (tea.Model, tea.Cmd) {
	w.st.mu.Lock()
	w.flushLocked()
	w.st.mu.Unlock()

	tm := trace.Begin()
	next, cmd := w.inner.Update(ev)
	d := tm.Elapsed()

	e := trace.Event{K: "msg", Op: fmt.Sprintf("%T", ev), Upd: d.Microseconds()}
	if k, ok := ev.(tea.KeyPressMsg); ok {
		e.Key = keyName(k)
	}
	if s, ok := next.(screener); ok {
		e.Screen = s.TraceScreen()
	}
	w.st.mu.Lock()
	w.st.pending = &e
	w.phaseAfterUpdate(e.Op)
	w.st.mu.Unlock()
	return Model{inner: next, st: w.st}, cmd
}

// View misura la View interna e chiude l'evento del messaggio precedente.
func (w Model) View() tea.View {
	tm := trace.Begin()
	v := w.inner.View()
	d := tm.Elapsed()

	w.st.mu.Lock()
	defer w.st.mu.Unlock()
	if !w.st.firstView {
		w.st.firstView = true
		trace.Emit(trace.Event{K: "phase", Op: "first_view"})
	}
	if w.st.listLoaded && !w.st.listView {
		w.st.listView = true
		trace.Emit(trace.Event{K: "phase", Op: "first_list_view"})
	}
	e := trace.Event{K: "msg", Op: "(view)"}
	if w.st.pending != nil {
		e = *w.st.pending
		w.st.pending = nil
	}
	e.View, e.ViewBytes = d.Microseconds(), len(v.Content)
	trace.Emit(e)
	return v
}

// flushLocked scrive l'evento in attesa senza View (stato st.mu tenuto).
func (w Model) flushLocked() {
	if w.st.pending != nil {
		trace.Emit(*w.st.pending)
		w.st.pending = nil
	}
}

// phaseAfterUpdate emette le fasi d'avvio riconosciute dal tipo del messaggio
// (connResult e msgsLoaded dell'app: il nome e stabile e coperto da un test di
// integrazione). st.mu tenuto.
func (w Model) phaseAfterUpdate(op string) {
	switch {
	case strings.HasSuffix(op, ".connResult") && !w.st.connected:
		w.st.connected = true
		trace.Emit(trace.Event{K: "phase", Op: "connected"})
	case strings.HasSuffix(op, ".msgsLoaded") && !w.st.listLoaded:
		w.st.listLoaded = true
		trace.Emit(trace.Event{K: "phase", Op: "list_loaded"})
	}
}

// keyName e il nome del tasto per la traccia: un tasto che produce caratteri
// e "char", mai il carattere; gli altri (frecce, invio, esc, ctrl+x, alt+x,
// tasti funzione) col loro nome. Un terminale col protocollo Kitty (Ghostty) puo
// consegnare il tasto stampabile senza Text, solo col codice: si tratta come testo
// anche un codice stampabile senza modificatori di comando.
func keyName(k tea.KeyPressMsg) string {
	const cmdMods = tea.ModCtrl | tea.ModAlt | tea.ModMeta | tea.ModSuper | tea.ModHyper
	if k.Text != "" || (k.Mod&cmdMods == 0 && unicode.IsPrint(k.Code)) {
		return "char"
	}
	return k.Keystroke()
}
