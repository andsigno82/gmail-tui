package app

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"gmail-tui/internal/mail"
	"gmail-tui/internal/msg"
	"gmail-tui/internal/theme"
	"gmail-tui/internal/ui/inbox"
	"gmail-tui/internal/ui/styles"
	"gmail-tui/internal/ui/thread"
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
	store     mail.MailStore
	styles    styles.Styles
	spinner   spinner.Model
	inbox     inbox.Model     // lista messaggi (Task 4): la radice delega tasti/mouse
	thread    thread.Model    // lettura messaggio (Task 5): costruito al msgLoaded
	search    textinput.Model // query di ricerca (Task 6): la radice delega i tasti
	label     textinput.Model // mini-prompt label (Task 7): render nella status bar
	labelOpen bool            // prompt label attivo: i tasti vanno all'input

	view       view
	refs       []msg.MessageRef // copia dei ref correnti (uso interno/test)
	pendingRef msg.MessageRef   // ref aperto in attesa della thread view (Task 5)
	query      msg.Query        // query corrente della lista (refresh dopo op, Task 7)
	queryRaw   string           // stringa digitata della query corrente ("" = inbox)
	status     statusMsg        // ultimo messaggio di status (effimero)
	pending    int              // op store in flight (>0: spinner attivo)
	width      int
	height     int
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
		inbox:   inbox.New(nil, st),
		search:  newSearchInput(st),
		label:   newLabelInput(st),
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
			// La query distingue l'errore di ricerca da quello di list.
			if ev.query != "" {
				m.status = statusMsg{text: fmt.Sprintf("ricerca %q: %v", ev.query, ev.err), isErr: true}
			} else {
				m.status = statusMsg{text: fmt.Sprintf("lettura messaggi: %v", ev.err), isErr: true}
			}
			return m, nil
		}
		// Il refresh dopo un'op (silent) deve ritrovare il cursore: la
		// selezione va catturata PRIMA di SetRefs (gli item vengono
		// sostituiti) — id e indice della riga corrente.
		var selID msg.MessageID
		hadSel := false
		oldIdx := 0
		if ev.silent {
			oldIdx = m.inbox.SelectedIndex()
			if r, ok := m.inbox.SelectedRef(); ok {
				selID, hadSel = r.ID, true
			}
		}
		m.refs = ev.refs
		if !ev.silent {
			if ev.query != "" {
				// Badge dei risultati di ricerca: conteggio + query digitata.
				m.status = statusMsg{text: fmt.Sprintf("%d risultati per %q", len(ev.refs), ev.query)}
			} else {
				// Re-list esplicito (load di Init o ricerca con query vuota):
				// il badge di una ricerca precedente è ormai stante, lo
				// status si azzera (Task 6 review).
				m.status = statusMsg{}
			}
		}
		// L'inbox riceve i ref direttamente qui (update di campo): con
		// bubbles/list v2 SetItems muta lo stato interno e la cmd restituita
		// è nil col filtraggio disabilitato — una cmd dedicata non aggiunge
		// nulla. La cmd viene comunque propagata per fedeltà all'API.
		cmd := m.inbox.SetRefs(ev.refs)
		if ev.silent && len(ev.refs) > 0 {
			// Ripristino del cursore dopo il refresh di un'op: stesso
			// messaggio se c'è ancora nell'elenco (anche se si è spostato),
			// altrimenti lo stesso indice clampato alla nuova lunghezza (il
			// messaggio operato può essere sparito, es. archiviato).
			idx := -1
			if hadSel {
				for i, r := range ev.refs {
					if r.ID == selID {
						idx = i
						break
					}
				}
			}
			if idx < 0 {
				idx = min(oldIdx, len(ev.refs)-1)
			}
			m.inbox.Select(idx)
		}
		return m, cmd

	case msgLoaded:
		m.doneOp()
		if ev.err != nil {
			m.status = statusMsg{text: fmt.Sprintf("apertura messaggio: %v", ev.err), isErr: true}
			return m, nil
		}
		if ev.m == nil {
			// Difensiva (il contratto MailStore non prevede (nil, nil)):
			// senza guardia thread.New dereferenzerebbe un puntatore nullo.
			m.status = statusMsg{text: "apertura messaggio: risposta vuota dallo store", isErr: true}
			return m, nil
		}
		// Il fetch è riuscito: si costruisce la thread view vera, dimensionata
		// se la size del terminale è già nota (WindowSizeMsg arriva all'avvio).
		m.thread = thread.New(m.pendingRef, ev.m, m.styles)
		if m.width > 0 {
			m.thread.SetSize(m.width, max(0, m.height-1))
		}
		// Auto MarkRead sul primo fetch di un messaggio NON letto: cmd
		// concatenata, errore SOLO in status (non fatale). La copia locale
		// del ref passa a letta subito; refresh=false (Task 5): niente
		// re-list, la lista si aggiorna via Watch/nuova ricerca, il back
		// preserva la lista.
		if ev.m.Ref.Unread {
			m.thread.MarkRead()
			return m, markRead(m.store, ev.m.Ref.ID, true, false)
		}
		return m, nil

	case opDone:
		m.doneOp()
		if ev.err != nil {
			m.status = statusMsg{text: fmt.Sprintf("%s: %v", ev.label, ev.err), isErr: true}
			return m, nil
		}
		m.status = statusMsg{text: ev.label}
		if ev.refresh {
			// Mutazione utente riuscita (Task 7): rilista la lista corrente
			// (query corrente) in silenzio — refs e cursore aggiornati, il
			// feedback resta in status.
			m.pending++
			return m, tea.Batch(refreshRefs(m.store, m.query, m.queryRaw), m.spinner.Tick)
		}
		return m, nil

	case changeEvt:
		// Il consumo vero (refresh guidato + indicatore disconnessione)
		// arriva in Task 8: qui l'evento viene solo ricevuto e ignorato.
		_ = ev.c
		return m, nil

	case inbox.SelectedMsg:
		// Apertura dal thread: si custodisce il ref e si fetcha fuori dal
		// loop UI (spinner attivo finché msgLoaded non arriva).
		m.pendingRef = ev.Ref
		m.view = viewThread
		m.pending++ // fetch in flight
		return m, tea.Batch(fetchMsg(m.store, ev.Ref.ID), m.spinner.Tick)

	case thread.BackMsg:
		// Uscita dal thread: si torna all'inbox così com'era — nessun
		// re-list, la lista conserva ref, cursore e stato.
		m.view = viewInbox
		return m, nil

	case thread.OpenRequested:
		// `o` nel thread: il tmpfile HTML è pronto (o la preparazione è
		// fallita). La exec vera sospende la TUI via tea.ExecProcess.
		if ev.Err != nil {
			m.status = statusMsg{text: fmt.Sprintf("apertura browser: %v", ev.Err), isErr: true}
			return m, nil
		}
		return m, thread.OpenCmd(ev.Path)

	case thread.OpenedMsg:
		// Il browser è uscito: solo feedback in status bar.
		if ev.Err != nil {
			m.status = statusMsg{text: fmt.Sprintf("browser: %v", ev.Err), isErr: true}
			return m, nil
		}
		m.status = statusMsg{text: "aperto nel browser"}
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
		// Il corpo occupa tutte le righe tranne l'ultima (status bar).
		m.inbox.SetSize(ev.Width, max(0, m.height-1))
		m.thread.SetSize(ev.Width, max(0, m.height-1))
		// L'input di ricerca non trabocca: il prompt "/ " occupa 2 colonne.
		m.search.SetWidth(max(0, ev.Width-2))
		// Idem il mini-prompt label: il prompt occupa len(labelPrompt) colonne.
		m.label.SetWidth(max(0, ev.Width-len(labelPrompt)))
		return m, nil

	case tea.KeyPressMsg:
		// Nota v2: KeyMsg è un'interfaccia press|release; si matchano solo le
		// pressioni (KeyPressMsg) per non duplicare le azioni.
		if m.labelOpen {
			// Mini-prompt label attivo (Task 7): TUTTI i tasti vanno al
			// prompt — enter applica, esc annulla, il resto è testo (i tasti
			// op non possono scattare mentre si digita il nome).
			switch ev.String() {
			case "enter":
				return m.confirmLabel()
			case "esc":
				return m.cancelLabel()
			default:
				var cmd tea.Cmd
				m.label, cmd = m.label.Update(ev)
				return m, cmd
			}
		}
		if m.view == viewSearch {
			// In ricerca SOLO enter ed esc sono della radice: ogni altro tasto
			// (incluso "q") è testo della query e va al textinput.
			switch ev.String() {
			case "enter":
				return m.submitSearch()
			case "esc":
				// Annulla: si torna all'inbox senza cercare; la bozza resta
				// nell'input per un eventuale ritorno con /.
				m.view = viewInbox
				m.search.Blur()
				return m, nil
			default:
				var cmd tea.Cmd
				m.search, cmd = m.search.Update(ev)
				return m, cmd
			}
		}
		switch ev.String() {
		case "a", "s", "v", "l":
			// Mutazioni (Task 7): agiscono sul ref corrente — nell'inbox la
			// riga selezionata, nel thread il messaggio aperto.
			return m.runOp(ev.String())
		case "q", "esc":
			if m.view == viewInbox {
				return m, tea.Quit // tea.Quit in v2 è una Msg-function: usata come Cmd
			}
			if m.view == viewThread {
				// Nel thread esc/q sono delegati al modello, che risponde con
				// BackMsg (la radice torna all'inbox al messaggio).
				var cmd tea.Cmd
				m.thread, cmd = m.thread.Update(ev)
				return m, cmd
			}
			return m, nil
		case "/":
			if m.view == viewInbox {
				m.view = viewSearch
				// Focus del textinput: la cmd avvia il blink del cursore.
				return m, m.search.Focus()
			}
		case "c":
			if m.view == viewInbox {
				m.status = statusMsg{text: "composer in Piano 4"}
				return m, nil
			}
		}
		// Le altre pressioni sono delegati alla vista attiva: nell'inbox alla
		// lista (j/k, frecce, paginazione; enter è gestito dall'inbox →
		// SelectedMsg), nel thread alla viewport (j/k, `o`).
		if m.view == viewInbox {
			var cmd tea.Cmd
			m.inbox, cmd = m.inbox.Update(ev)
			return m, cmd
		}
		if m.view == viewThread {
			var cmd tea.Cmd
			m.thread, cmd = m.thread.Update(ev)
			return m, cmd
		}
		return m, nil

	case tea.MouseClickMsg, tea.MouseWheelMsg:
		// Mouse first-class: nell'inbox la lista gestisce rotella e click
		// (selezione; doppio click → SelectedMsg); nel thread la viewport
		// gestisce la rotella nativamente.
		if m.view == viewInbox {
			var cmd tea.Cmd
			m.inbox, cmd = m.inbox.Update(ev)
			return m, cmd
		}
		if m.view == viewThread {
			var cmd tea.Cmd
			m.thread, cmd = m.thread.Update(ev)
			return m, cmd
		}
		return m, nil

	default:
		// In ricerca anche i messaggi non-tasto (es. blink del cursore del
		// textinput) passano al modello: senza, il cursore resterebbe fermo.
		if m.view == viewSearch {
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(ev)
			return m, cmd
		}
		// Idem per il mini-prompt label mentre è aperto (blink del cursore).
		if m.labelOpen {
			var cmd tea.Cmd
			m.label, cmd = m.label.Update(ev)
			return m, cmd
		}
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

// submitSearch esce dalla vista ricerca ed esegue la query corrente: la
// ricerca (o il re-list, se vuota) gira in Cmd e l'inbox riceverà i
// risultati via msgsLoaded. L'input si azzera: il prossimo / parte pulito.
// La query si custodisce sul modello: è lo stato del refresh silenzioso che
// segue le mutazioni (Task 7) — il re-list gira con la query corrente.
func (m Model) submitSearch() (Model, tea.Cmd) {
	raw := strings.TrimSpace(m.search.Value())
	m.view = viewInbox
	m.search.Blur()
	m.search.Reset()
	m.queryRaw = raw
	m.query = msg.Query{}
	if raw != "" {
		m.query = msg.ParseQuery(raw)
	}
	m.pending++ // ricerca/re-list in flight: spinner finché msgsLoaded
	return m, tea.Batch(submitQuery(m.store, raw), m.spinner.Tick)
}

// loadRefs esegue ListMessages fuori dal loop UI. Il contesto è background
// finché il wiring del programma non esponga il ctx del programma (Task 7/8).
func loadRefs(st mail.MailStore) tea.Cmd {
	return func() tea.Msg {
		refs, err := st.ListMessages(context.Background(), msg.Query{}, mail.Page{Limit: 50})
		return msgsLoaded{refs: refs, err: err}
	}
}

// fetchMsg esegue FetchMessage fuori dal loop UI (risultato: msgLoaded).
func fetchMsg(st mail.MailStore, id msg.MessageID) tea.Cmd {
	return func() tea.Msg {
		mm, err := st.FetchMessage(context.Background(), id)
		return msgLoaded{m: mm, err: err}
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

// body rende la vista attiva: l'inbox è la lista messaggi (Task 4), il
// thread è la lettura del messaggio (Task 5, placeholder durante il fetch),
// la ricerca è il textinput della query (Task 6).
func (m Model) body() string {
	switch m.view {
	case viewThread:
		if !m.thread.Loaded() {
			return "Caricamento messaggio…"
		}
		return m.thread.View()
	case viewSearch:
		return m.searchBody()
	default:
		return m.inbox.View()
	}
}

// statusBar rende l'ultima riga: il mini-prompt label quando è aperto (Task
// 7), altrimenti spinner se pending > 0, ultimo statusMsg, hint tasti quando
// non c'è nulla da dire.
func (m Model) statusBar() string {
	if m.labelOpen {
		// Il prompt è stilizzato dall'input (SearchInput/ListDate): rende sé
		// stesso al posto dello status, che resta custodito sotto.
		return m.label.View()
	}
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
