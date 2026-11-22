// Wizard nel radice (Piano 4, Task 5): la vista first-run mostrata dal Model
// quando il main attiva la modalità wizard (config mancante E nessuna password
// risolvibile, o `gmail-tui login`). Il radice delega TUTTI i tasti al wizard
// e NON avvia load/feed finché il wizard non completa (CompletedMsg):
//   - skip (`s`) → demo mode con il seed di app.DemoStore() (store E sender
//     un solo oggetto), identity mittente demo e banner;
//   - successo → la factory (BackendFactory, iniettata dal main: imap+smtp
//     preset Gmail) costruisce i backend reali con le credenziali VERIFICATE
//     dal wizard, l'identity viene impostata (firma vuota al primo setup) e
//     parte il flusso normale. Gli errori NON fatali del salvataggio
//     (config/keychain) finiscono nello status d'avvio: il setup completa
//     comunque, con il suggerimento GMAIL_APP_PASSWORD per il keychain.
package app

import (
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/config"
	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/ui/wizard"
)

// BackendFactory costruisce store e sender REALI con le credenziali verificate
// dal wizard (il main passa imap.NewStore + smtp.NewSender con preset Gmail;
// né store né sender si connettono qui: il dial avviene alla prima operazione).
// Seam per i test, che passano una factory finta.
type BackendFactory func(address, password string) (mail.MailStore, Sender)

// SetWizard attiva la modalità first-run (Piano 4, Task 5): la vista wizard
// sostituisce l'inbox finché CompletedMsg, e il radice usa la factory per
// costruire i backend reali al successo. Il load iniziale di Init NON parte in
// questa modalità (pending torna a 0): store e sender correnti sono il
// placeholder passato al costruttore — il main passa app.DemoStore() — e il
// load vero parte al completamento (skip → il placeholder è sostituito dal
// seed; successo → dalla factory). Da chiamare PRIMA di Init/Run.
func (m *Model) SetWizard(w wizard.Model, factory BackendFactory) {
	m.wiz = w
	m.factory = factory
	m.wizardActive = true
	m.view = viewWizard
	m.pending = 0 // niente load in flight: parte al completamento
}

// wizardCompleted gestisce il CompletedMsg del wizard (skip o setup riuscito):
// chiude la modalità wizard, costruisce la sessione giusta e avvia il flusso
// normale (load + spinner + feed Watch). Un CompletedMsg STANTE (il wizard è
// già stato chiuso — doppio `s` in volo) è scartato.
func (m Model) wizardCompleted(ev wizard.CompletedMsg) (tea.Model, tea.Cmd) {
	if !m.wizardActive {
		return m, nil
	}
	// Piano 7.2, fix review: il wizard conserva nome, firma e preferenze nella
	// config: il radice le riprende, così un `,` successivo non le scrive vuote.
	saved := m.savedSettings()
	m.SetPrefs(saved.Layout, saved.PreviewMarkRead)
	// La factory va CONSUMATA prima di azzerarla (serve qui sotto per i
	// backend reali): la si salva localmente e si azzera lo stato del radice.
	factory := m.factory
	m.wizardActive = false
	m.wiz = wizard.Model{}
	m.factory = nil
	m.view = viewInbox

	if ev.Skipped {
		// Demo mode col seed (Piano 4, Task 5): store e sender sono UN solo
		// Mock popolato — chi clona il repo vede l'app viva. L'identity è il
		// destinatario dei seed ("tu@example.com"): senza SetIdentity il
		// composer aperto col primo `c` nasce col campo Da vuoto.
		mk := DemoStore()
		m.store, m.sender = mk, mk
		m.SetIdentity("tu@example.com", "", "")
		m.SetDemoBanner()
	} else {
		// Setup riuscito: backend reali con le credenziali verificate. La
		// firma parte vuota al primo setup (config con signature vuota —
		// l'utente la aggiungerà dopo).
		if factory != nil {
			m.store, m.sender = factory(ev.Address, ev.Password)
		}
		m.SetIdentity(ev.Address, saved.Name, saved.Signature)
		switch {
		case ev.ConfigErr != nil:
			// Config non salvata (percorso non scrivibile): la sessione
			// funziona ma al prossimo avvio non ritroverà l'indirizzo. Errore
			// visibile, non fatale.
			m.startup = true
			m.status = statusMsg{
				text:  fmt.Sprintf("configurazione completata — config NON salvata: %v", ev.ConfigErr),
				isErr: true,
			}
		case ev.KeyringErr != nil:
			// Keychain che fallisce (o assente): la password non sopravvive al
			// processo — il piano chiede lo status che indica la env var.
			m.startup = true
			m.status = statusMsg{
				text:  fmt.Sprintf("configurazione completata — password NON salvata nel keychain (%v) — imposta GMAIL_APP_PASSWORD per il prossimo avvio", ev.KeyringErr),
				isErr: true,
			}
		default:
			// Tutto liscio: feedback positivo in stile StatusOK, che sopravvive
			// al primo load (startup).
			m.startup = true
			m.status = statusMsg{text: "configurazione completata — connesso come " + ev.Address, isOK: true}
		}
	}
	m.pending++ // il load della sessione parte ora: spinner finché msgsLoaded
	return m, m.beginSession()
}

// savedSettings legge la config salvata (m.cfgPath). Config assente o non
// leggibile → default; valori non validi → riportati al default per campo.
func (m Model) savedSettings() config.Config {
	cfg, err := config.Load(m.cfgPath)
	if err != nil && !errors.Is(err, config.ErrInvalid) {
		return config.Default()
	}
	return cfg.Sanitized()
}

// wizardBody è il corpo della vista wizard (il View del modello del wizard).
func (m Model) wizardBody() string {
	return m.wiz.View()
}
