// Annulla invio (Piano 9, Task 9, D9-I): `s` nella conferma chiude il composer
// e avvia una finestra di 5 secondi; allo scadere parte l'invio. `u` fuori dal
// composer riporta il composer com'era. Se l'app si chiude nella finestra,
// l'invio parte prima dell'uscita (quitCmd). Timer e flush condividono lo
// stesso pendingSend, reclamato una sola volta: niente doppio invio.
package app

import (
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/composer"
	"github.com/andsigno82/gmail-tui/internal/ui/toast"
)

// undoWindow è la finestra di annullamento dell'invio (D9-I). I test non la
// aspettano: consegnano direttamente undoDueMsg col seq del pendingSend.
const undoWindow = 5 * time.Second

// pendingSend è un invio confermato in attesa: il composer è custodito per il
// ripristino con `u`. taken è condiviso fra le copie del Model (il puntatore).
type pendingSend struct {
	seq     int
	out     msg.Outgoing
	draftID msg.MessageID
	comp    composer.Model
	back    view   // vista di provenienza del composer (ritorno dopo l'annullamento)
	tmp     string // directory degli allegati della bozza ripresa (fix review)
	keep    bool
	taken   atomic.Bool
}

// claim reclama l'invio: vero una sola volta (timer o flush in uscita).
func (p *pendingSend) claim() bool { return p.taken.CompareAndSwap(false, true) }

// undoDueMsg è lo scadere della finestra; seq stantio → ignorato.
type undoDueMsg struct{ seq int }

// undoTimer arma il timer della finestra di annullamento.
func undoTimer(seq int) tea.Cmd {
	return tea.Tick(undoWindow, func(time.Time) tea.Msg { return undoDueMsg{seq: seq} })
}

// startUndo mette in attesa l'invio confermato: il composer si chiude (torna la
// vista di provenienza), toast e timer. m.draft è la bozza normalizzata.
func (m *Model) startUndo() tea.Cmd {
	m.undoSeq++
	p := &pendingSend{seq: m.undoSeq, out: m.draft, draftID: m.draftID, comp: m.composer, back: m.composeBack, tmp: m.draftTmp, keep: m.draftKeep}
	m.undo = p
	m.draft = msg.Outgoing{}
	m.closeComposer()
	toastCmd := m.toast.Push(toast.Info, "invio tra 5 s — u annulla")
	return tea.Batch(toastCmd, undoTimer(p.seq))
}

// restoreUndo è `u` fuori dal composer: il composer torna com'era prima della
// conferma (corpo, campi, bozza salvata) e la vista corrente diventa il ritorno.
func (m *Model) restoreUndo() tea.Cmd {
	p := m.undo
	m.undo = nil
	if !p.claim() {
		return nil
	}
	m.reopenPending(p)
	cmd := m.toast.Push(toast.Info, "invio annullato: bozza ripristinata")
	return cmd
}

// reopenPending riapre il composer con la bozza di un invio in attesa: usato da
// `u` e quando l'invio fallisce dopo la finestra (la bozza non va persa).
func (m *Model) reopenPending(p *pendingSend) {
	m.composer = p.comp
	m.composeBack = p.back
	m.draftID = p.draftID
	m.draftTmp = p.tmp
	m.draftKeep = p.keep
	m.draftBusy = false
	m.discardOpen = false
	m.view = viewComposer
	if m.width > 0 {
		m.composer.SetSize(m.width, max(0, m.height-1))
	}
}
