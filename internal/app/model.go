package app

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"gmail-tui/internal/config"
	"gmail-tui/internal/mail"
	"gmail-tui/internal/msg"
	"gmail-tui/internal/theme"
	"gmail-tui/internal/ui/composer"
	"gmail-tui/internal/ui/inbox"
	"gmail-tui/internal/ui/styles"
	"gmail-tui/internal/ui/thread"
)

// view identifica la vista attiva (routing del Model radice).
type view int

const (
	viewInbox    view = iota // lista messaggi (Task 4)
	viewThread               // lettura thread (Task 5)
	viewSearch               // ricerca (Task 6)
	viewComposer             // scrittura messaggio (Piano 4, Task 3)
)

// Model è la radice della TUI: routing viste, status bar globale, keymap
// centrale (Task 9: definizioni in keys.go, help via bubbles/help). Tutte le
// chiamate MailStore e Sender girano in tea.Cmd — la rete non blocca mai.
//
// Shutdown (mandato Task 8): q/esc nell'inbox e ctrl+c da qualunque stato
// escono puliti via quitCmd, che cancella il ctx del feed Watch PRIMA del
// tea.QuitMsg. LIMITAZIONE nota: una SIGINT esterna (kill -INT, ctrl+c fuori
// da raw mode) arriva al runtime come tea.InterruptMsg e NON passa da Update —
// il ctx del feed non viene annullato in quel percorso; il goroutine resta
// parcheggiato fino all'uscita del processo (innocuo per una CLI). La
// intercettazione via WithFilter è stata scartata in review (Task 8).
type Model struct {
	store     mail.MailStore
	sender    Sender // invio composer (P4 Task 4): Mock in demo, SMTP altrimenti
	styles    styles.Styles
	spinner   spinner.Model
	help      help.Model      // barra breve + overlay `?` (Task 9): stili a tema
	inbox     inbox.Model     // lista messaggi (Task 4): la radice delega tasti/mouse
	thread    thread.Model    // lettura messaggio (Task 5): costruito al msgLoaded
	composer  composer.Model  // scrittura messaggio (P4 Task 3): costruito al r/f/c
	search    textinput.Model // query di ricerca (Task 6): la radice delega i tasti
	label     textinput.Model // mini-prompt label (Task 7): render nella status bar
	labelOpen bool            // prompt label attivo: i tasti vanno all'input
	showHelp  bool            // overlay help completo aperto (Task 9): i tasti sono ignorati
	demo      bool            // demo mode (Task 9): banner iniziale in stile DemoBanner

	// Composer (Piano 4, Task 3-4): identity mittente/firma (SetIdentity,
	// seam per il wiring dalla config del main), vista di provenienza per il
	// back, conferma discard aperta, bozza in attesa di conferma invio
	// (SendRequestedMsg → conferma modale → Sender) e conferma invio aperta.
	from, signature string
	composeBack     view
	discardOpen     bool
	draft           msg.Outgoing
	confirmSend     bool // conferma pre-invio aperta ("Invio a …" nella status bar)

	// Percorso della config per la persistenza del tema (Piano 4, Task 2:
	// `:theme`): risolto in New da config.Path(). "" = non disponibile
	// (os.UserConfigDir in errore): il tema si applica a runtime ma non si
	// salva. SetConfigPath lo sostituisce (seam main/test).
	cfgPath string

	view       view
	refs       []msg.MessageRef // copia dei ref correnti (uso interno/test)
	pendingRef msg.MessageRef   // ref aperto in attesa della thread view (Task 5)
	query      msg.Query        // query corrente della lista (refresh dopo op, Task 7)
	queryRaw   string           // stringa digitata della query corrente ("" = inbox)
	status     statusMsg        // ultimo messaggio di status (effimero)
	startup    bool             // status d'avvio (banner demo/warning tema): sopravvive al primo load
	pending    int              // op store in flight (>0: spinner attivo)
	refreshing bool             // re-list silenzioso in flight (coalescing, Task 8)
	dirty      bool             // cambio arrivato durante il refresh: follow-up al risultato (Task 8)

	// Ctx del feed Watch (Task 8): creato in New, annullato dal quitCmd
	// all'uscita pulita (Task 9: q/esc/ctrl+c). Una SIGINT esterna invece
	// bypassa l'Update (tea.InterruptMsg, limitazione documentata sopra).
	ctx     context.Context
	cancel  context.CancelFunc
	watchCh <-chan mail.Change // canale del feed aperto dall'Init (via watchStarted)

	width  int
	height int
}

// Conformance: il Model radice implementa tea.Model (v2: View rende tea.View).
var _ tea.Model = Model{}

// Sender è l'astrazione di invio del composer (Piano 4, Task 4): la UI parla
// SOLO con questa interfaccia, mai con SMTP. È soddisfatta da *smtp.Sender
// (modalità reale) e da *mail.Mock (demo mode: store e sender sono un solo
// oggetto) — assert di conformance nel main e nei test dell'app.
type Sender interface {
	Send(ctx context.Context, m msg.Outgoing) error
}

// New costruisce il Model radice per lo store, il sender e il tema dati. Il
// ctx del feed Watch nasce qui (Task 8): l'Init lo passa a Watch e il loop
// waitForChange muore quando viene annullato — il wiring del cancel all'uscita
// del programma è del Task 9 (main); nei test il cancel è esplicito.
func New(store mail.MailStore, sender Sender, th theme.Theme) Model {
	st := styles.New(th)
	ctx, cancel := context.WithCancel(context.Background())
	// Percorso config per la persistenza del tema (Piano 4, Task 2): con
	// UserConfigDir non disponibile resta "" e `:theme` non persiste (tema
	// runtime comunque applicato). I test lo sostituiscono con SetConfigPath.
	var cfgPath string
	if p, err := config.Path(); err == nil {
		cfgPath = p
	}
	return Model{
		store:   store,
		sender:  sender,
		styles:  st,
		spinner: spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(st.Spinner)),
		help:    newHelp(st),
		inbox:   inbox.New(nil, st),
		search:  newSearchInput(st),
		label:   newLabelInput(st),
		cfgPath: cfgPath,
		view:    viewInbox,
		ctx:     ctx,
		cancel:  cancel,
		// Il load iniziale emesso sempre da Init è già in flight: pending
		// parte da 1 così la status bar mostra lo spinner finché msgsLoaded
		// non arriva. Le op successive incrementeranno al dispatch (Task 6/7).
		pending: 1,
	}
}

// Init emette il load iniziale della lista (Query vuota, prima pagina da 50),
// avvia lo spinner e apre il feed Watch (Task 8): al watchStarted la radice
// arma il loop waitForChange che guida il refresh automatico.
func (m Model) Init() tea.Cmd {
	return tea.Batch(loadRefs(m.store), m.spinner.Tick, startWatch(m.store, m.ctx))
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
		// Il token del re-list silenzioso si libera a QUALSIASI esito (un
		// refresh fallito non deve bloccare i successivi); dirty NON viene
		// consumato: il follow-up partirà alla prima riuscita (self-healing).
		m.refreshing = false
		if ev.err != nil {
			// La query distingue l'errore di ricerca da quello di list.
			if ev.query != "" {
				m.status = statusMsg{text: fmt.Sprintf("ricerca %q: %v", ev.query, ev.err), isErr: true}
			} else {
				m.status = statusMsg{text: fmt.Sprintf("lettura messaggi: %v", ev.err), isErr: true}
			}
			return m, nil
		}
		// L'indicatore di riconnessione (Task 8) si spegne al primo msgsLoaded
		// riuscito che non lo abbia già sostituito (op, badge di ricerca): la
		// riconnessione è andata a buon fine e la lista è fresca.
		if m.status.text == reconnectIndicator && !m.status.isErr {
			m.status = statusMsg{}
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
			} else if m.startup {
				// Status d'avvio (banner demo o warning tema, Task 9): il
				// load di Init NON lo cancella — resta finché un evento vero
				// non lo sostituisce. Il flag si consuma qui: i load
				// successivi (ricerca con query vuota) tornano ad azzerare.
				m.startup = false
			} else {
				// Re-list esplicito (ricerca con query vuota): il badge di
				// una ricerca precedente è ormai stante, lo status si
				// azzera (Task 6 review).
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
		// Coalescing (Task 8): se un cambio è arrivato mentre il refresh era
		// in volo, il follow-up parte ora (lo snapshot appena consumato è
		// comunque più vecchio del cambio).
		var follow tea.Cmd
		if m.dirty {
			follow = m.requestRefresh()
		}
		return m, tea.Batch(cmd, follow)

	case msgLoaded:
		m.doneOp()
		// Guardia anti-race (fix merge-gate): il risultato porta l'ID
		// richiesto. Se non coincide col ref in attesa (open A lenta → esc →
		// open B veloce: il risultato di A arriva per ultimo) o non siamo più
		// in thread (esc durante il fetch), il risultato è STANTE — si ignora
		// del tutto: niente rebuild della thread, niente auto MarkRead,
		// niente status. Il doneOp sopra è comunque giusto: ogni fetch
		// incrementa pending e la sua risposta lo chiude.
		if ev.requested != m.pendingRef.ID || m.view != viewThread {
			return m, nil
		}
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
		if ev.closeComposer {
			// Invio riuscito (Piano 4, Task 4): il composer si chiude e si
			// torna all'inbox (come da piano); il feedback "inviato" resta in
			// status e il refresh sotto rilista la lista corrente.
			m.finishSend()
		}
		if ev.refresh {
			// Mutazione utente riuscita (Task 7): rilista la lista corrente
			// (query corrente) in silenzio — refs e cursore aggiornati, il
			// feedback resta in status. Via requestRefresh (Task 8): con un
			// refresh già in volo non si accatasta, il follow-up parte al
			// risultato (dirty).
			return m, m.requestRefresh()
		}
		return m, nil

	case watchStarted:
		// Apertura del feed Watch (Task 8): si custodisce il canale e si
		// arma il loop waitForChange. Con errore solo feedback in status: la
		// TUI resta usabile (re-list espliciti, nessun push).
		if ev.err != nil {
			m.status = statusMsg{text: fmt.Sprintf("watch: %v", ev.err), isErr: true}
			return m, nil
		}
		m.watchCh = ev.ch
		return m, waitForChange(m.ctx, ev.ch)

	case changeEvt:
		// Evento push del feed (Task 8): re-list silenzioso guidato dal tipo
		// di cambio + loop ri-armato (handleChange in watch.go).
		return m.handleChange(ev)

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

	case thread.SaveRequested:
		// `:save`/`:open` della command line (Piano 4, Task 2): il salvataggio
		// gira fuori dal loop UI (spinner finché attachmentSaved); il feedback
		// — "salvato in …" (StatusBar; StatusOK adottato in Task 5) o l'exec
		// del browser per :open — arriva al risultato.
		m.pending++
		return m, tea.Batch(saveAttachment(m.store, ev), m.spinner.Tick)

	case attachmentSaved:
		m.doneOp()
		if ev.err != nil {
			m.status = statusMsg{text: fmt.Sprintf("salva allegato: %v", ev.err), isErr: true}
			return m, nil
		}
		if ev.open {
			// `:open`: il tmpfile col nome originale è pronto — stessa exec
			// del tasto `o`; il feedback viene da OpenedMsg.
			return m, thread.OpenCmd(ev.path)
		}
		m.status = statusMsg{text: "salvato in " + ev.path}
		return m, nil

	case thread.CommandFeedback:
		// Esito dei comandi senza effetto store (elenco temi, uso errato,
		// comando sconosciuto): direttamente in status bar.
		m.status = statusMsg{text: ev.Text, isErr: ev.IsErr}
		return m, nil

	case thread.ThemeChanged:
		// `:theme <nome>` (Piano 4, Task 2): la UI cambia SUBITO — stili della
		// radice, spinner, help e le due sub-viste (SetStyles ri-renderizza in
		// place header/corpo/allegati del thread e il delegate dell'inbox,
		// preservando stato: ref, scroll, lista). La persistenza gira in cmd
		// (saveThemeCmd): errore → themePersisted → status, tema intatto.
		m.styles = ev.St
		m.spinner.Style = ev.St.Spinner
		m.help = newHelp(ev.St)
		m.inbox.SetStyles(ev.St)
		m.thread.SetStyles(ev.St)
		m.status = statusMsg{text: fmt.Sprintf("tema %s applicato", ev.Th.Name)}
		return m, saveThemeCmd(m.cfgPath, ev.Th.Name)

	case themePersisted:
		// Persistenza del tema riuscita: il feedback "tema … applicato" resta.
		// Fallita: solo status d'errore, il tema runtime non si perde.
		if ev.err != nil {
			m.status = statusMsg{text: fmt.Sprintf("salvataggio tema: %v", ev.err), isErr: true}
		}
		return m, nil

	case composeLoaded:
		// Risultato del fetch per r/f nell'inbox (Piano 4, Task 3). Come per
		// msgLoaded: i fetch non hanno ordine garantito — se l'utente ha
		// cambiato vista nel frattempo (es. ha aperto un messaggio) il
		// risultato è STANTE e si ignora (nessun yank nel composer); il
		// doneOp sopra chiude comunque l'op.
		m.doneOp()
		if m.view != viewInbox {
			return m, nil
		}
		if ev.err != nil {
			label := "risposta"
			if ev.forward {
				label = "inoltro"
			}
			m.status = statusMsg{text: fmt.Sprintf("%s: %v", label, ev.err), isErr: true}
			return m, nil
		}
		if ev.m == nil {
			// Difensiva (il contratto MailStore non prevede (nil, nil)).
			m.status = statusMsg{text: "risposta: messaggio vuoto dallo store", isErr: true}
			return m, nil
		}
		cfg := replyFor(ev.ref, ev.m)
		if ev.forward {
			cfg = forwardFor(ev.ref, ev.m)
		}
		m.openComposer(cfg)
		return m, nil

	case composer.SendRequestedMsg:
		// ctrl+s nel composer (Piano 4, Task 4): la bozza viene custodita e
		// si apre la conferma modale ("Invio a …" nella status bar): s →
		// sendCmd col Sender, esc → ripresa (bozza intatta). Messaggio STANTE
		// (la vista è cambiata fra pressione e consegna): scartato, come i
		// risultati di fetch stanti.
		if m.view != viewComposer {
			return m, nil
		}
		m.draft = ev.Out
		m.confirmSend = true
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
		if m.view == viewComposer {
			// Il composer è la vista attiva: ridimensiona campi e corpo
			// (meno l'ultima riga, riservata alla status bar).
			m.composer.SetSize(ev.Width, max(0, m.height-1))
		}
		// L'input di ricerca non trabocca: il prompt "/ " occupa 2 colonne.
		m.search.SetWidth(max(0, ev.Width-2))
		// Idem il mini-prompt label: il prompt occupa len(labelPrompt) colonne.
		m.label.SetWidth(max(0, ev.Width-len(labelPrompt)))
		// La barra help breve si tronca alla larghezza disponibile (Task 9).
		m.help.SetWidth(ev.Width)
		return m, nil

	case tea.KeyPressMsg:
		// Nota v2: KeyMsg è un'interfaccia press|release; si matchano solo le
		// pressioni (KeyPressMsg) per non duplicare le azioni.
		if ev.String() == "ctrl+c" {
			// Uscita immediata da qualunque stato (inbox, ricerca, prompt,
			// overlay): quitCmd cancella il ctx del feed e esce. Il routing
			// resta identico a Task 3-8 per tutti gli altri tasti (keymap
			// centrale in keys.go, Task 9).
			return m, m.quitCmd()
		}
		if m.showHelp {
			// Overlay help aperto (Task 9): ?/esc chiudono, gli altri tasti
			// sono ignorati — nessuna azione "alla cieca" sotto l'overlay.
			switch ev.String() {
			case "?", "esc":
				m.showHelp = false
			}
			return m, nil
		}
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
		if m.discardOpen {
			// Conferma discard del composer (Piano 4, Task 3): s scarta la
			// bozza, esc riprende — gli altri tasti sono ignorati (nessuna
			// azione "alla cieca", come sotto l'overlay help).
			switch ev.String() {
			case "s":
				m.discardDraft()
				return m, nil
			case "esc":
				m.discardOpen = false
			}
			return m, nil
		}
		if m.confirmSend {
			// Conferma pre-invio del composer (Piano 4, Task 4): s consegna la
			// bozza al Sender fuori dal loop UI (spinner finché opDone), esc
			// riprende il composer con la bozza intatta — gli altri tasti (e
			// il mouse, gate sotto) sono ignorati: nessuna azione "alla cieca".
			switch ev.String() {
			case "s":
				m.confirmSend = false
				m.pending++ // invio in flight: spinner finché opDone
				return m, tea.Batch(sendCmd(m.sender, m.draft), m.spinner.Tick)
			case "esc":
				m.confirmSend = false // back al composer: la bozza resta
			}
			return m, nil
		}
		if m.view == viewThread && m.thread.ModalOpen() {
			// Modalità comando/filepicker del thread (Piano 4, Task 2):
			// TUTTI i tasti vanno al thread — enter esegue/conferma, esc
			// annulla, il resto è testo dell'input o navigazione del picker
			// (j/k non scorrono, a/s/v/l non operano, come col prompt label).
			var cmd tea.Cmd
			m.thread, cmd = m.thread.Update(ev)
			return m, cmd
		}
		if m.view == viewComposer {
			// Composer attivo (Piano 4, Task 3): TUTTI i tasti vanno al
			// composer — esc chiede il cancel (back diretto o conferma
			// discard), tab/frecce navigano i campi, il resto è testo: i
			// tasti op dell'app (r/f/a/…) non possono scattare mentre si
			// scrive. La richiesta di cancel viene consumata QUI: corpo
			// vuoto → back diretto, corpo pieno → conferma discard.
			var cmd tea.Cmd
			m.composer, cmd = m.composer.Update(ev)
			if m.composer.CancelRequested() {
				m.handleComposerCancel()
				return m, nil
			}
			return m, cmd
		}
		if m.view == viewSearch {
			// In ricerca SOLO enter ed esc sono della radice: ogni altro tasto
			// (incluso "q" e "?") è testo della query e va al textinput.
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
		// Il routing usa la keymap centrale (Task 9): stesse chiavi e stesso
		// ordine del vecchio switch su ev.String() (a/s/v/l → q/esc → …).
		switch {
		case key.Matches(ev, keys.Reply, keys.Forward):
			// r/f (Piano 4, Task 3): nell'inbox fetch-then-compose (cache-
			// first via store.FetchMessage), nel thread composer immediato
			// dal messaggio già caricato. Il composer precompilato sostituisce
			// la vista corrente (il back del composer torna lì).
			return m.startCompose(ev.String() == "f")
		case key.Matches(ev, keys.Archive, keys.Star, keys.Read, keys.Label):
			// Mutazioni (Task 7): agiscono sul ref corrente — nell'inbox la
			// riga selezionata, nel thread il messaggio aperto.
			return m.runOp(ev.String())
		case key.Matches(ev, keys.Quit):
			if m.view == viewInbox {
				// Uscita pulita (Task 9): cancel del feed Watch + QuitMsg.
				return m, m.quitCmd()
			}
			if m.view == viewThread {
				// Nel thread esc/q sono delegati al modello, che risponde con
				// BackMsg (la radice torna all'inbox al messaggio).
				var cmd tea.Cmd
				m.thread, cmd = m.thread.Update(ev)
				return m, cmd
			}
			return m, nil
		case key.Matches(ev, keys.Help):
			// Overlay help completo (Task 9): si apre sopra il corpo, ?/esc
			// lo chiudono (gestiti in testa al case KeyPressMsg).
			m.showHelp = true
			return m, nil
		case key.Matches(ev, keys.Search):
			if m.view == viewInbox {
				m.view = viewSearch
				// Focus del textinput: la cmd avvia il blink del cursore.
				return m, m.search.Focus()
			}
		case key.Matches(ev, keys.Composer):
			if m.view == viewInbox {
				// c (Piano 4, Task 3): nuovo messaggio — il composer vuoto
				// (identity via SetIdentity) sostituisce la vista corrente.
				m.openComposer(newMessage(config.Config{}))
				return m, nil
			}
		case key.Matches(ev, keys.Command):
			// `:` nel thread (Piano 4, Task 2): catturato dalla radice e
			// delegato al thread, che apre la command line (:save/:open/:theme
			// — cmdmode.go). Nelle altre viste non significa nulla: la key
			// cade nella delega di coda senza effetti.
			if m.view == viewThread {
				var cmd tea.Cmd
				m.thread, cmd = m.thread.Update(ev)
				return m, cmd
			}
		}
		// Le altre pressioni sono delegati alla vista attiva: nell'inbox alla
		// lista (j/k, frecce, paginazione; enter è gestito dall'inbox →
		// SelectedMsg), nel thread alla viewport (j/k, `o`), nel composer al
		// campo focalizzato (gate sopra).
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
		// Con un modale aperto il mouse è ignorato, come i tasti (fix
		// merge-gate): sotto l'overlay help un doppio click aprirebbe un
		// messaggio invisibile dietro l'overlay; col prompt label un click
		// muoverebbe il cursore dell'inbox e enter applicherebbe la label
		// alla riga sbagliata (rompendo l'invariante di confirmLabel); con la
		// command line/filepicker del thread (Piano 4, Task 2) click e rotella
		// non devono toccare ciò che c'è dietro il modale; nel composer
		// (P4 Task 3) il mouse non tocca i campi (keyboard-first) e sotto le
		// conferme discard/invio (P4 Task 4) è ignorato come i tasti.
		if m.showHelp || m.labelOpen || m.discardOpen || m.confirmSend || m.thread.ModalOpen() {
			return m, nil
		}
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
		// Idem per i modali del thread (Piano 4, Task 2): blink della command
		// line e readDirMsg del filepicker (risultato della sua Init) devono
		// raggiungere il thread, altrimenti il picker non carica la directory.
		if m.view == viewThread && m.thread.ModalOpen() {
			var cmd tea.Cmd
			m.thread, cmd = m.thread.Update(ev)
			return m, cmd
		}
		// Idem per il composer (Piano 4, Task 3): blink dei cursori dei
		// textinput/textarea e readDirMsg del filepicker allegati devono
		// raggiungere il modello.
		if m.view == viewComposer {
			var cmd tea.Cmd
			m.composer, cmd = m.composer.Update(ev)
			return m, cmd
		}
		return m, nil
	}
}

// View compone il corpo della vista attiva e la status bar finale, riempiendo
// lo schermo fino all'ultima riga (quando la size è nota); sotto il minimo
// 80×24 rende la schermata di avviso dedicata (Task 9). Dichiarano qui anche
// le capability per-View di tea v2: altscreen (TUI a schermo intero) e mouse
// cell motion (click + rotella: tutto ciò che list/viewport custom usano —
// AllMotion aggiungerebbe solo traffico hover).
func (m Model) View() tea.View {
	v := tea.NewView(m.content())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
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
// L'ID richiesto viaggia DENTRO il risultato: i fetch non hanno ordine
// garantito e il handler lo usa per scartare le risposte stanti (fix
// merge-gate anti-race).
func fetchMsg(st mail.MailStore, id msg.MessageID) tea.Cmd {
	return func() tea.Msg {
		mm, err := st.FetchMessage(context.Background(), id)
		return msgLoaded{m: mm, err: err, requested: id}
	}
}

// Min-size della TUI (Task 9): sotto queste dimensioni la View mostra la
// schermata di avviso dedicata (View-only: Update resta attivo, q esce comunque).
const (
	minWidth  = 80
	minHeight = 24
)

// demoBannerText è il banner di avvio in demo mode (nessuna GMAIL_APP_PASSWORD,
// Task 9): status iniziale impostato dal main via SetDemoBanner, reso con lo
// stile DemoBanner finché un evento vero non lo sostituisce.
const demoBannerText = "demo mode — set GMAIL_APP_PASSWORD"

// SetDemoBanner attiva la modalità demo (chiamato dal main PRIMA di avviare il
// programma, quando lo store è il Mock): lo status iniziale diventa il banner,
// che sopravvive al load di Init e resta finché il prossimo evento (op,
// ricerca, Watch) non lo sostituisce.
func (m *Model) SetDemoBanner() {
	m.demo = true
	m.startup = true
	m.status = statusMsg{text: demoBannerText}
}

// SetStartupWarning imposta un warning d'avvio (es. tema di fallback, Task 9):
// status normale (non errore), sostituisce l'eventuale banner demo — il
// problema più specifico vince. Come il banner, sopravvive al load di Init.
func (m *Model) SetStartupWarning(text string) {
	m.startup = true
	m.status = statusMsg{text: text}
}

// SetConfigPath sostituisce il percorso della config usato dalla persistenza
// del tema (`:theme`, Piano 4, Task 2): seam per i test (config finta in
// t.TempDir) e per il main, che può volere un percorso esplicito. Il default
// risolto in New è config.Path().
func (m *Model) SetConfigPath(path string) {
	m.cfgPath = path
}

// content rende corpo + status bar, con il corpo esteso fino alla riga sopra
// la barra (la barra resta sull'ultima riga del terminale). Con il terminale
// sotto il minimo rende SOLO la schermata di avviso (Task 9).
func (m Model) content() string {
	if m.tooSmall() {
		return m.minSizeView()
	}
	bar := m.statusBar()
	body := m.body()
	if m.showHelp {
		// Overlay help completo (Task 9): sostituisce il corpo, la status bar
		// resta (l'help breve è sempre visibile).
		body = m.helpOverlay()
	}
	if m.height > 1 {
		lines := 1 + strings.Count(body, "\n")
		if pad := m.height - 1 - lines; pad > 0 {
			return body + strings.Repeat("\n", pad) + "\n" + bar
		}
	}
	return body + "\n" + bar
}

// tooSmall dice se la finestra è sotto il minimo 80×24 (solo quando la size è
// nota: prima del primo WindowSizeMsg la TUI rende il contenuto normale).
func (m Model) tooSmall() bool {
	return m.width > 0 && (m.width < minWidth || m.height < minHeight)
}

// minSizeView è la schermata di avviso per i terminali troppo piccoli (Task
// 9, View-only): mostra la size corrente e il minimo richiesto. Nessun corpo
// né status bar: non c'è nulla di sensato da mostrare a quelle dimensioni.
func (m Model) minSizeView() string {
	body := m.styles.ThreadHeader.Render("gmail-tui") + "\n\n" +
		fmt.Sprintf("terminale troppo piccolo: %d×%d\n", m.width, m.height) +
		fmt.Sprintf("servono almeno %d×%d colonne\n", minWidth, minHeight) +
		m.styles.ListDate.Render("allarga la finestra — q esce comunque")
	return m.styles.Border.Render(body)
}

// helpOverlay rende l'help completo (toggle `?`, Task 9): il titolo con
// l'hint di chiusura e le colonne dei gruppi (bubbles/help FullHelpView) in
// un riquadro a tema.
func (m Model) helpOverlay() string {
	title := m.styles.ThreadHeader.Render("aiuto") +
		m.styles.ListDate.Render("  esc/? chiudi")
	return title + "\n" +
		m.styles.Border.Render(m.styles.HelpView.Render(m.help.FullHelpView(m.fullHelpFor(m.labelOpen, m.view))))
}

// body rende la vista attiva: l'inbox è la lista messaggi (Task 4), il
// thread è la lettura del messaggio (Task 5, placeholder durante il fetch),
// la ricerca è il textinput della query (Task 6), il composer i campi di
// scrittura (Piano 4, Task 3).
func (m Model) body() string {
	switch m.view {
	case viewThread:
		if !m.thread.Loaded() {
			return "Caricamento messaggio…"
		}
		return m.thread.View()
	case viewSearch:
		return m.searchBody()
	case viewComposer:
		return m.composer.View()
	default:
		return m.inbox.View()
	}
}

// statusBar rende l'ultima riga: il mini-prompt label quando è aperto (Task
// 7), altrimenti lo spinner se pending > 0, l'ultimo statusMsg e — sempre
// visibile, Task 9 — l'help breve per la vista corrente (bubbles/help). Un
// errore riempie la barra da solo (visibilità prima dell'help); il banner
// demo usa lo stile DemoBanner.
func (m Model) statusBar() string {
	if m.labelOpen {
		// Il prompt è stilizzato dall'input (SearchInput/ListDate): rende sé
		// stesso al posto dello status, che resta custodito sotto.
		return m.label.View()
	}
	if m.discardOpen {
		// Conferma discard del composer (Piano 4, Task 3): occupa la barra
		// per intero (stile StatusErr: azione distruttiva, ben visibile).
		return m.renderBar(m.styles.StatusErr, "scartare la bozza? s conferma · esc annulla")
	}
	if m.confirmSend {
		// Conferma pre-invio del composer (Piano 4, Task 4): occupa la barra
		// per intero — destinatari decodificati dalla bozza, oggetto e numero
		// di allegati (testo in sendConfirmText, compose.go).
		return m.renderBar(m.styles.StatusErr, sendConfirmText(m.draft))
	}
	text := m.status.text
	if m.pending > 0 {
		text = m.spinner.View() + " " + text
	}
	if m.status.isErr {
		return m.renderBar(m.styles.StatusErr, text)
	}
	if m.demo && m.status.text == demoBannerText {
		return m.renderBar(m.styles.DemoBanner, text)
	}
	return m.renderBar(m.styles.StatusBar, m.joinHelp(text))
}

// joinHelp affianca al testo di status l'help breve (sempre visibile, Task 9):
// a status vuota resta solo l'help (sostituisce il vecchio hint fisso); con
// la size nota l'help si spinge a destra e si tronca allo spazio disponibile.
func (m Model) joinHelp(text string) string {
	hv := m.help
	if m.width > 0 {
		// Il padding della barra (0,1) occupa 2 colonne.
		hv.SetWidth(max(0, m.width-lipgloss.Width(text)-2))
	}
	helpBar := hv.ShortHelpView(m.shortHelpFor(m.labelOpen, m.view))
	if text == "" {
		return helpBar
	}
	if helpBar == "" {
		return text
	}
	gap := 2
	if m.width > 0 {
		if spare := m.width - lipgloss.Width(text) - lipgloss.Width(helpBar) - 2; spare > gap {
			gap = spare
		}
	}
	return text + strings.Repeat(" ", gap) + helpBar
}

// renderBar applica lo stile dato (a larghezza piena quando la size è nota).
func (m Model) renderBar(st lipgloss.Style, s string) string {
	if m.width > 0 {
		st = st.Width(m.width)
	}
	return st.Render(s)
}

// currentView è un helper per i test: la vista attiva.
func (m Model) currentView() view { return m.view }

// statusText è un helper per i test: il testo corrente della status bar.
func (m Model) statusText() string { return m.status.text }
