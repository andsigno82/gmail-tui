// Package wizard contiene il first-run wizard della TUI (Piano 4, Task 5):
// la schermata di configurazione guidata mostrata dal radice quando la config
// MANCA e nessuna password è risolvibile (env/keychain) — o forzata da
// `gmail-tui login`. Macchina a stati: welcome (istruzioni 2FA + URL delle
// App Password, apribile con `o`) → input indirizzo → input password
// (mascherato) → verifica di connessione REALE (funzione iniettata dal main,
// con spinner e budget di 30s; errore → messaggio chiaro e retry) → salvataggio
// config + keyring → CompletedMsg col risultato per il radice.
//
// Il modello NON tocca IMAP né il keychain vero: le dipendenze arrivano da
// Deps (funzione di verifica iniettata, keyring.Store finto nei test, percorso
// config in t.TempDir) — nessuna rete nei test. `s` al welcome salta: il
// radice entra in demo mode col seed (app.DemoStore()).
package wizard

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/config"
	"github.com/andsigno82/gmail-tui/internal/keyring"
	"github.com/andsigno82/gmail-tui/internal/ui/styles"
)

// Step è il passo corrente della macchina a stati del wizard. CompletedMsg
// chiude la macchina: il radice smette di renderizzare il wizard e passa
// all'inbox (demo o reale).
type Step int

const (
	StepWelcome  Step = iota // istruzioni 2FA + URL, invio/s/o
	StepAddress              // input indirizzo Gmail
	StepPassword             // input password applicativa (mascherato)
	StepTesting              // verifica connessione in flight (spinner)
	StepSaving               // salvataggio config + keyring in flight
)

// appPasswordURL è l'URL di gestione delle Google App Password, mostrato nel
// welcome e apribile con `o`.
const appPasswordURL = "https://myaccount.google.com/apppasswords"

// connectTimeout è il budget della verifica di connessione (Piano 4, Task 5:
// "timeout 30s"). Il wizard avvolge il ctx prima di chiamare TestConnect, così
// il vincolo vale per QUALSIASI implementazione iniettata.
const connectTimeout = 30 * time.Second

// hint dei passi (il resto della UI del wizard è testo pieno).
const (
	addressHint  = "invio continua · esc indietro"
	passwordHint = "invio verifica la connessione · esc indietro"
)

// Deps sono le dipendenze del wizard: la verifica di connessione REALE
// (iniettata dal main con imap.NewStore+Connect+ListMessages; nei test una
// finta — nessuna rete), il keychain (nil = non disponibile: nessun Set e il
// completamento suggerisce la env var) e il percorso dove salvare la config
// ("" = nessun salvataggio possibile).
type Deps struct {
	TestConnect func(ctx context.Context, address, password string) error
	Keyring     keyring.Store
	CfgPath     string
}

// CompletedMsg chiude il wizard e porta al radice tutto il necessario per
// costruire la sessione:
//   - Skipped: `s` al welcome — il radice entra in demo mode col seed;
//   - Address/Password: le credenziali VERIFICATE (verifica riuscita) — il
//     radice le passa alla factory dei backend reali;
//   - ConfigErr/KeyringErr: errori NON fatali del salvataggio (il wizard
//     completa comunque; il radice li mostra come status d'avvio, con il
//     suggerimento GMAIL_APP_PASSWORD per il keychain).
type CompletedMsg struct {
	Skipped    bool
	Address    string
	Password   string
	ConfigErr  error
	KeyringErr error
}

// connTested è l'esito della verifica di connessione (interno al wizard).
type connTested struct{ err error }

// urlOpened è l'esito dell'exec del browser sull'URL (interno).
type urlOpened struct{ err error }

// Model è il wizard. Gli input sono textinput bubbles (indirizzo in chiaro,
// password mascherata con EchoPassword); lo spinner gira nei passi testing
// e saving (i suoi tick sono delegati dal radice mentre il wizard è attivo).
type Model struct {
	st       styles.Styles
	deps     Deps
	step     Step
	address  textinput.Model
	password textinput.Model
	spinner  spinner.Model
	addr, pw string // credenzali congelate all'avvio della verifica (retry stabile)
	err      error  // ultimo errore della verifica/url (rappresentato in View)
	errText  string // errore di validazione degli input (testo, non error)
	width    int
}

// New costruisce il wizard al primo passo (welcome). La cmd di blink del
// focus iniziale non serve: al welcome non c'è input focalizzato.
func New(deps Deps, st styles.Styles) Model {
	m := Model{
		st:   st,
		deps: deps,
		step: StepWelcome,
	}
	m.address = newInput(st, "nome@gmail.com", false)
	m.password = newInput(st, "password applicativa (16 caratteri)", true)
	m.spinner = spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(st.Spinner))
	return m
}

// newInput costruisce un textinput del wizard: placeholder attenuato, testo
// normale, cursore blocco. masked=true (EchoPassword + EchoCharacter '*')
// per la password applicativa.
func newInput(st styles.Styles, placeholder string, masked bool) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 128
	if masked {
		ti.EchoMode = textinput.EchoPassword
		ti.EchoCharacter = '*'
	}
	var s textinput.Styles
	s.Focused.Placeholder = st.ListDate
	s.Focused.Text = st.ListNormal
	s.Blurred.Placeholder = st.ListDate
	s.Blurred.Text = st.ListNormal
	s.Cursor = textinput.CursorStyle{Shape: tea.CursorBlock, Blink: true}
	ti.SetStyles(s)
	return ti
}

// Step è il passo corrente (seam per i test e per il help del radice).
func (m Model) Step() Step { return m.step }

// Init avvia lo spinner del wizard (i passi testing/saving lo mostrano).
func (m Model) Init() tea.Cmd { return m.spinner.Tick }

// SetWidth dimensiona gli input alla larghezza del terminale (il radice la
// propaga al WindowSizeMsg).
func (m *Model) SetWidth(w int) {
	m.width = w
	m.address.SetWidth(max(0, w-4))
	m.password.SetWidth(max(0, w-4))
}

// Update instrada i messaggi: tasti (per passo), esiti delle Cmd, tick dello
// spinner e blink dei textinput (i messaggi non-tasto raggiungono l'input
// focalizzato del passo corrente).
func (m Model) Update(ev tea.Msg) (Model, tea.Cmd) {
	mm := m
	switch ev := ev.(type) {
	case connTested:
		// Esito della verifica: errore → back al password con messaggio
		// (retry con invio); successo → salvataggio config+keyring in cmd.
		if ev.err != nil {
			mm.step = StepPassword
			mm.err = ev.err
			return mm, nil
		}
		mm.step = StepSaving
		mm.err = nil
		return mm, mm.finalizeCmd()

	case urlOpened:
		if ev.err != nil {
			mm.err = ev.err
		}
		return mm, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		mm.spinner, cmd = mm.spinner.Update(ev)
		if mm.step != StepTesting && mm.step != StepSaving {
			return mm, nil // spinner fermo fuori dai passi con spinner
		}
		return mm, cmd

	case tea.KeyPressMsg:
		return mm.updateKey(ev)
	}

	// Messaggi non-tasto (blink del cursore, ecc.): all'input del passo.
	var cmd tea.Cmd
	switch mm.step {
	case StepAddress:
		mm.address, cmd = mm.address.Update(ev)
	case StepPassword:
		mm.password, cmd = mm.password.Update(ev)
	}
	return mm, cmd
}

// updateKey gestisce i tasti per il passo corrente. Nei passi di attesa
// (testing/saving) QUALSIASI tasto è ignorato: l'operazione è in flight e
// l'esito arriverà come connTested/CompletedMsg.
func (m Model) updateKey(k tea.KeyPressMsg) (Model, tea.Cmd) {
	switch m.step {
	case StepWelcome:
		switch k.String() {
		case "enter":
			m.err, m.errText = nil, ""
			m.step = StepAddress
			return m, m.address.Focus()
		case "o":
			return m, openURLCmd()
		case "s":
			// Salta: demo mode col seed (la scelta è del radice al msg).
			return m, func() tea.Msg { return CompletedMsg{Skipped: true} }
		}
		return m, nil

	case StepAddress:
		switch k.String() {
		case "enter":
			addr := strings.TrimSpace(m.address.Value())
			if addr == "" {
				m.errText = "inserisci l'indirizzo Gmail (es. nome@gmail.com)"
				return m, nil
			}
			if !strings.Contains(addr, "@") {
				m.errText = "indirizzo non valido: manca @"
				return m, nil
			}
			m.err, m.errText = nil, ""
			m.addr = addr
			m.step = StepPassword
			return m, m.password.Focus()
		case "esc":
			m.err, m.errText = nil, ""
			m.step = StepWelcome
			return m, nil
		}
		var cmd tea.Cmd
		m.address, cmd = m.address.Update(k)
		return m, cmd

	case StepPassword:
		switch k.String() {
		case "enter":
			pw := m.password.Value()
			if pw == "" {
				m.errText = "inserisci la password applicativa (16 caratteri)"
				return m, nil
			}
			m.err, m.errText = nil, ""
			m.pw = pw
			m.step = StepTesting
			return m, tea.Batch(m.testConnCmd(), m.spinner.Tick)
		case "esc":
			m.err, m.errText = nil, ""
			m.step = StepAddress
			return m, nil
		}
		var cmd tea.Cmd
		m.password, cmd = m.password.Update(k)
		return m, cmd
	}

	// StepTesting / StepSaving: niente tasti (attesa).
	return m, nil
}

// testConnCmd avvia la verifica di connessione REALE fuori dal loop UI: ctx
// con il budget connectTimeout (il vincolo vale per ogni implementazione
// iniettata). L'esito torna come connTested.
func (m Model) testConnCmd() tea.Cmd {
	return func() tea.Msg {
		if m.deps.TestConnect == nil {
			return connTested{err: errors.New("verifica di connessione non configurata")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
		defer cancel()
		return connTested{err: m.deps.TestConnect(ctx, m.addr, m.pw)}
	}
}

// finalizeCmd persiste la config e la password nel keychain fuori dal loop UI
// (Piano 4, Task 5: successo = config.Save con address/name/signature vuote/
// tema default + keyring.Set). Entrambi gli errori sono NON fatali: il
// CompletedMsg li porta al radice e il wizard completa comunque (la sessione
// corrente funziona con le credenziali in memoria).
func (m Model) finalizeCmd() tea.Cmd {
	return func() tea.Msg {
		var cfgErr, krErr error
		if m.deps.CfgPath != "" {
			cfg := config.Config{Address: m.addr, Theme: "default"}
			cfgErr = cfg.Save(m.deps.CfgPath)
		}
		if m.deps.Keyring != nil {
			krErr = m.deps.Keyring.Set(keyring.ServiceName, m.addr, m.pw)
		} else {
			krErr = errors.New("keychain non disponibile: imposta GMAIL_APP_PASSWORD per conservare la password")
		}
		return CompletedMsg{Address: m.addr, Password: m.pw, ConfigErr: cfgErr, KeyringErr: krErr}
	}
}

// openURLCmd apre l'URL delle App Password nel browser di sistema (stesso
// meccanismo del tasto `o` del thread): exec che sospende la TUI, esito in
// urlOpened per l'eventuale messaggio d'errore.
func openURLCmd() tea.Cmd {
	return tea.ExecProcess(browserCmd(appPasswordURL), func(err error) tea.Msg {
		return urlOpened{err: err}
	})
}

// browserCmd sceglie il comando di sistema per aprire un URL: open su darwin,
// xdg-open altrove (come thread.browserCmd per i file HTML).
func browserCmd(url string) *exec.Cmd {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", url)
	}
	return exec.Command("xdg-open", url)
}

// View rende il passo corrente: welcome (istruzioni + URL), input (con
// messaggio d'errore sotto), attesa (spinner). La riga d'uso è in coda.
func (m Model) View() string {
	var b strings.Builder
	errLine := ""
	switch {
	case m.errText != "":
		errLine = m.st.StatusErr.Render(m.errText)
	case m.err != nil:
		errLine = m.st.StatusErr.Render(fmt.Sprintf("errore: %v", m.err))
	}

	switch m.step {
	case StepWelcome:
		b.WriteString(m.st.ThreadHeader.Render("Benvenuto in gmail-tui") + "\n\n")
		b.WriteString(m.st.ListNormal.Render("Per connetterti a Gmail serve una password applicativa:") + "\n")
		b.WriteString(m.st.ListNormal.Render("1. attiva la verifica in due passaggi sul tuo account Google") + "\n")
		b.WriteString(m.st.ListNormal.Render("2. crea una password applicativa dedicata a gmail-tui") + "\n\n")
		b.WriteString(m.st.SearchInput.Render(appPasswordURL))
		b.WriteString(m.st.ListDate.Render("  — premi o per aprirla nel browser") + "\n")
		if errLine != "" {
			// Errore dell'apertura del browser (`o`): visibile senza uscire dal welcome.
			b.WriteString("\n" + errLine + "\n")
		}
		b.WriteString("\n" + m.st.HelpView.Render("invio continua · s salta (modalità demo) · ctrl+c esci"))
	case StepAddress:
		b.WriteString(m.st.ThreadHeader.Render("Configurazione — indirizzo") + "\n\n")
		b.WriteString(m.st.ThreadHeaderKey.Render("Indirizzo Gmail: ") + m.address.View() + "\n")
		if errLine != "" {
			b.WriteString("\n" + errLine + "\n")
		}
		b.WriteString("\n" + m.st.HelpView.Render(addressHint))
	case StepPassword:
		b.WriteString(m.st.ThreadHeader.Render("Configurazione — password") + "\n\n")
		b.WriteString(m.st.ThreadHeaderKey.Render("Password applicativa: ") + m.password.View() + "\n")
		if errLine != "" {
			b.WriteString("\n" + errLine + "\n")
		}
		b.WriteString("\n" + m.st.HelpView.Render(passwordHint))
	case StepTesting:
		b.WriteString("\n" + m.spinner.View() + " " + m.st.ListNormal.Render("verifica connessione a imap.gmail.com…") + "\n\n")
		b.WriteString(m.st.HelpView.Render("attendi l'esito della verifica"))
	case StepSaving:
		b.WriteString("\n" + m.spinner.View() + " " + m.st.ListNormal.Render("salvataggio configurazione…") + "\n\n")
		b.WriteString(m.st.HelpView.Render("attendi il completamento"))
	}
	return b.String()
}
