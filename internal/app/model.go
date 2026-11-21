package app

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"gmail-tui/internal/mail"
	"gmail-tui/internal/msg"
	"gmail-tui/internal/theme"
	"gmail-tui/internal/ui/styles"
)

// view identifica la vista attiva (routing del Model radice).
type view int

const (
	viewInbox  view = iota // lista messaggi (Task 4)
	viewThread             // lettura thread (Task 5)
	viewSearch             // ricerca (Task 6)
)

// Model è la radice della TUI: routing viste, status bar globale, keymap.
// Tutte le chiamate MailStore girano in tea.Cmd — la rete non blocca mai.
type Model struct {
	store   mail.MailStore
	styles  styles.Styles
	spinner spinner.Model

	view    view
	refs    []msg.MessageRef // righe della lista corrente (inbox Task 4)
	status  statusMsg        // ultimo messaggio di status (effimero)
	pending int              // op store in flight (>0: spinner attivo)
	width   int
	height  int
}

// Conformance: il Model radice implementa tea.Model (v2: View rende tea.View).
var _ tea.Model = Model{}

// New costruisce il Model radice per lo store e il tema dati.
func New(store mail.MailStore, th theme.Theme) Model {
	st := styles.New(th)
	return Model{
		store:   store,
		styles:  st,
		spinner: spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(st.Spinner)),
		view:    viewInbox,
		// Il load iniziale emesso sempre da Init è già in flight: pending
		// parte da 1 così la status bar mostra lo spinner finché msgsLoaded
		// non arriva. Le op successive incrementeranno al dispatch (Task 6/7).
		pending: 1,
	}
}

// Init emette il load iniziale della lista (Query vuota, prima pagina da 50)
// e avvia lo spinner, che resta attivo finché pending > 0.
func (m Model) Init() tea.Cmd {
	return tea.Batch(loadRefs(m.store), m.spinner.Tick)
}

// Update gestisce i messaggi: risultati delle Cmd, tick dello spinner, tasti
// di routing e resize del terminale.
func (m Model) Update(ev tea.Msg) (tea.Model, tea.Cmd) {
	switch ev := ev.(type) {
	case statusMsg:
		// Timeout logico: lo status resta finché il prossimo evento non lo
		// sostituisce; nessun timer.
		m.status = ev
		return m, nil

	case msgsLoaded:
		m.doneOp()
		if ev.err != nil {
			m.status = statusMsg{text: fmt.Sprintf("lettura messaggi: %v", ev.err), isErr: true}
			return m, nil
		}
		m.refs = ev.refs
		return m, nil

	case msgLoaded:
		m.doneOp()
		if ev.err != nil {
			m.status = statusMsg{text: fmt.Sprintf("apertura messaggio: %v", ev.err), isErr: true}
			return m, nil
		}
		// Il consumo vero (thread view) arriva in Task 5: finché non esiste
		// la vista il messaggio caricato non ha effetto visivo.
		_ = ev.m
		return m, nil

	case opDone:
		m.doneOp()
		if ev.err != nil {
			m.status = statusMsg{text: fmt.Sprintf("%s: %v", ev.label, ev.err), isErr: true}
			return m, nil
		}
		m.status = statusMsg{text: ev.label}
		return m, nil

	case changeEvt:
		// Il consumo vero (refresh guidato + indicatore disconnessione)
		// arriva in Task 8: qui l'evento viene solo ricevuto e ignorato.
		_ = ev.c
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(ev)
		if m.pending <= 0 {
			return m, nil // niente op in flight: spinner fermo, non ri-armare
		}
		return m, cmd

	case tea.WindowSizeMsg:
		m.width, m.height = ev.Width, ev.Height
		return m, nil

	case tea.KeyPressMsg:
		// Nota v2: KeyMsg è un'interfaccia press|release; si matchano solo le
		// pressioni (KeyPressMsg) per non duplicare le azioni.
		switch ev.String() {
		case "q", "esc":
			if m.view == viewInbox {
				return m, tea.Quit // tea.Quit in v2 è una Msg-function: usata come Cmd
			}
			m.view = viewInbox // in thread/search si torna alla lista
			return m, nil
		case "/":
			if m.view == viewInbox {
				m.view = viewSearch
				return m, nil
			}
		case "c":
			if m.view == viewInbox {
				m.status = statusMsg{text: "composer in Piano 4"}
				return m, nil
			}
		}
		return m, nil

	default:
		return m, nil
	}
}

// View compone il corpo della vista attiva e la status bar finale, riempiendo
// lo schermo fino all'ultima riga (quando la size è nota).
func (m Model) View() tea.View {
	return tea.NewView(m.content())
}

// doneOp chiude un'op store in flight (non sotto zero).
func (m *Model) doneOp() {
	if m.pending > 0 {
		m.pending--
	}
}

// loadRefs esegue ListMessages fuori dal loop UI. Il contesto è background
// finché il wiring del programma non esponga il ctx del programma (Task 7/8).
func loadRefs(st mail.MailStore) tea.Cmd {
	return func() tea.Msg {
		refs, err := st.ListMessages(context.Background(), msg.Query{}, mail.Page{Limit: 50})
		return msgsLoaded{refs: refs, err: err}
	}
}

// content rende corpo + status bar, con il corpo esteso fino alla riga sopra
// la barra (la barra resta sull'ultima riga del terminale).
func (m Model) content() string {
	bar := m.statusBar()
	body := m.body()
	if m.height > 1 {
		lines := 1 + strings.Count(body, "\n")
		if pad := m.height - 1 - lines; pad > 0 {
			return body + strings.Repeat("\n", pad) + "\n" + bar
		}
	}
	return body + "\n" + bar
}

// body rende la vista attiva. Inbox/thread/search sono stub di scheletro:
// le viste vere arrivano nei Task 4-6.
func (m Model) body() string {
	switch m.view {
	case viewThread:
		return "Thread — stub (Task 5)"
	case viewSearch:
		return "Ricerca — stub (Task 6)"
	default:
		title := "Posta in arrivo"
		if n := len(m.refs); n > 0 {
			title += fmt.Sprintf(" — %d messaggi", n)
		}
		return title
	}
}

// statusBar rende l'ultima riga: spinner se pending > 0, ultimo statusMsg,
// hint tasti quando non c'è nulla da dire.
func (m Model) statusBar() string {
	text := m.status.text
	if m.pending > 0 {
		text = m.spinner.View() + " " + text
	} else if text == "" {
		text = "q esci · / cerca" // help contestuale vera in Task 9
	}
	st := m.styles.StatusBar
	if m.status.isErr {
		st = m.styles.StatusErr
	}
	if m.width > 0 {
		st = st.Width(m.width)
	}
	return st.Render(text)
}

// currentView è un helper per i test: la vista attiva.
func (m Model) currentView() view { return m.view }

// statusText è un helper per i test: il testo corrente della status bar.
func (m Model) statusText() string { return m.status.text }
