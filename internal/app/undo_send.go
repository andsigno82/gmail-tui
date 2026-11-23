// Annulla invio (Piano 9, Task 9, D9-I; review A): `s` nella conferma salva SUBITO
// il messaggio in Bozze (saveForSendCmd), poi chiude il composer e avvia la
// finestra di 5 secondi; allo scadere parte l'invio, e solo dopo l'invio riuscito
// la copia viene tolta da Bozze. `u` fuori dal composer riapre il composer sulla
// bozza già salvata. Se l'app si chiude nella finestra, l'invio parte prima
// dell'uscita (quitCmd). Timer e flush condividono lo stesso pendingSend,
// reclamato una sola volta: niente doppio invio.
package app

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/cmd"
	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/mail/smtp"
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
	draftID msg.MessageID // copia in Bozze del messaggio (da rimuovere dopo l'invio riuscito)
	replace []msg.MessageID
	comp    composer.Model
	back    view   // vista di provenienza del composer (ritorno dopo l'annullamento)
	tmp     string // directory degli allegati della bozza ripresa (fix review)
	keep    bool
	taken   atomic.Bool
	// saved si chiude quando il salvataggio in Bozze è finito (ok o errore): draftID
	// è scritto prima della chiusura, quindi leggerlo dopo <-saved è sicuro.
	saved chan struct{}
}

// sendSaved è l'esito del salvataggio in Bozze alla conferma (review A): la
// finestra di annullamento parte solo dopo questo messaggio. err: avviso, l'invio
// resta possibile.
type sendSaved struct {
	seq int
	err error
}

// errSaveUnconfirmed: in uscita il salvataggio non si è concluso nel tempo utile.
var errSaveUnconfirmed = errors.New("salvataggio in Bozze non confermato: controlla la cartella Bozze")

// claim reclama l'invio: vero una sola volta (timer o flush in uscita).
func (p *pendingSend) claim() bool { return p.taken.CompareAndSwap(false, true) }

// undoDueMsg è lo scadere della finestra; seq stantio → ignorato.
type undoDueMsg struct{ seq int }

// undoTimer arma il timer della finestra di annullamento.
func undoTimer(seq int) tea.Cmd {
	return tea.Tick(undoWindow, func(time.Time) tea.Msg { return undoDueMsg{seq: seq} })
}

// startUndo conferma l'invio (review A): il messaggio viene salvato in Bozze
// (saveForSendCmd) prima della finestra; l'esito sendSaved arma la finestra.
// Ripresa con allegati non scaricati (keep): nessuna copia, l'originale resta
// in Bozze, la finestra parte subito. m.draft è la bozza normalizzata.
func (m *Model) startUndo() tea.Cmd {
	m.undoSeq++
	m.lastUndo = nil // Piano 10, Task 6 (D10-U2): l'invio è l'ultima azione annullabile
	m.undoSendOff = false
	p := &pendingSend{seq: m.undoSeq, out: m.draft, draftID: m.draftID, comp: m.composer, back: m.composeBack, tmp: m.draftTmp, keep: m.draftKeep, saved: make(chan struct{})}
	m.undo = p
	m.draft = msg.Outgoing{}
	if m.draftKeep {
		close(p.saved)
		return m.armUndo(p)
	}
	if m.draftID != "" {
		p.replace = []msg.MessageID{m.draftID}
	}
	m.draftBusy = true
	return saveForSendCmd(m.store, p)
}

// saveForSendCmd salva il messaggio in Bozze fuori dal loop UI (review A): il MIME
// è smtp.BuildDraft, la copia sostituisce la bozza ripresa (replace). Con errore di
// rimozione della precedente l'ID nuovo resta comunque (review F).
func saveForSendCmd(st mail.MailStore, p *pendingSend) tea.Cmd {
	return func() tea.Msg {
		defer close(p.saved)
		raw, err := smtp.BuildDraft(p.out, time.Now())
		if err != nil {
			return sendSaved{seq: p.seq, err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), cmd.SendTimeout)
		defer cancel()
		id, err := st.SaveDraft(ctx, raw, p.replace)
		if id != "" {
			p.draftID = id
		}
		return sendSaved{seq: p.seq, err: err}
	}
}

// saveWait è il tempo massimo che l'invio attende il salvataggio in Bozze (budget
// del comando SMTP; variabile per i test).
var saveWait = cmd.SendTimeout

// awaitSave aspetta la fine del salvataggio in Bozze (in uscita o allo scadere).
// false: il salvataggio non si è concluso nel tempo utile, draftID non è sicuro.
func awaitSave(p *pendingSend) bool {
	select {
	case <-p.saved:
		return true
	case <-time.After(saveWait):
		return false
	}
}

// armUndo chiude il composer e avvia la finestra di 5 s (D9-I).
func (m *Model) armUndo(p *pendingSend) tea.Cmd {
	m.draftBusy = false
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

// exitLog raccoglie gli esiti dell'invio in uscita (review A): la TUI è chiusa
// quando succedono, quindi runProgram li stampa su stderr dopo p.Run().
type exitLog struct {
	mu    sync.Mutex
	notes []string
}

func (l *exitLog) add(s string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.notes = append(l.notes, s)
}

// ExitMessages restituisce gli esiti registrati in uscita (vuoto se nessuno).
func (m Model) ExitMessages() []string {
	if m.exit == nil {
		return nil
	}
	m.exit.mu.Lock()
	defer m.exit.mu.Unlock()
	return append([]string(nil), m.exit.notes...)
}
