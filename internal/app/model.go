package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/andsigno82/gmail-tui/internal/config"
	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/theme"
	"github.com/andsigno82/gmail-tui/internal/ui/composer"
	"github.com/andsigno82/gmail-tui/internal/ui/inbox"
	"github.com/andsigno82/gmail-tui/internal/ui/styles"
	"github.com/andsigno82/gmail-tui/internal/ui/thread"
	"github.com/andsigno82/gmail-tui/internal/ui/wizard"
)

// view identifica la vista attiva (routing del Model radice).
type view int

const (
	viewInbox    view = iota // lista messaggi (Task 4)
	viewThread               // lettura thread (Task 5)
	viewSearch               // ricerca (Task 6)
	viewComposer             // scrittura messaggio (Piano 4, Task 3)
	viewWizard               // first-run wizard (Piano 4, Task 5)
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

	discardOpen bool
	draft       msg.Outgoing
	confirmSend bool // conferma pre-invio aperta ("Invio a …" nella status bar)
	sending     bool // invio in flight (da conferma a opDone): blocca un secondo sendCmd

	// Impostazioni (Piano 7.2, Task 2-3): tema applicato (th), layout e segna
	// come letto in anteprima (SetPrefs), indirizzo e nome del mittente
	// (SetIdentity: from è derivato). menu è il modale `,` (settings.go).
	th       theme.Theme
	layout   string
	markRead string
	address  string
	name     string
	menu     settingsMenu

	// Vista a due colonne (Piano 7.2, Task 4): focus sull'anteprima (Tab),
	// messaggio sotto il cursore mostrato a destra (prevRef, valido con prevHas)
	// e token dei timer/fetch dell'anteprima (prevTok, split.go).
	focusPreview bool
	// lastPreviewClick: ultimo click sinistro sulla colonna dell'anteprima (doppio
	// click → thread a schermo intero, Piano 7.2, fix review).
	lastPreviewClick time.Time
	prevRef      msg.MessageRef
	prevHas      bool
	prevTok      int

	// Wizard first-run (Piano 4, Task 5): attivo finché CompletedMsg — tutti
	// i tasti vanno al wizard, il load/feed partono solo al completamento
	// (skip → DemoStore seed; successo → factory dei backend reali, app/wizard.go).
	wiz          wizard.Model
	wizardActive bool
	factory      BackendFactory

	// Percorso della config per la persistenza del tema (Piano 4, Task 2:
	// `:theme`): risolto in New da config.Path(). "" = non disponibile
	// (os.UserConfigDir in errore): il tema si applica a runtime ma non si
	// salva. SetConfigPath lo sostituisce (seam main/test).
	cfgPath string

	view          view
	refs          []msg.MessageRef // copia dei ref correnti (uso interno/test)
	pendingRef    msg.MessageRef   // ref aperto in attesa della thread view (Task 5)
	query         msg.Query        // query corrente della lista (refresh dopo op, Task 7)
	queryRaw      string           // stringa digitata della query corrente ("" = inbox)
	status        statusMsg        // ultimo messaggio di status (effimero)
	startup       bool             // status d'avvio (banner demo/warning tema): sopravvive al primo load
	pending       int              // op store in flight (>0: spinner attivo)
	connectFailed bool             // Connect d'avvio fallito: R riprova (bugfix 2026-11-22)
	refreshing    bool             // re-list silenzioso in flight (coalescing, Task 8)
	listGen       int              // generazione della lista: +1 a ogni load non silenzioso (cambio cartella/ricerca/connect); i risultati di altre generazioni sono stanti (Piano 7, fix review 2)
	refreshGen    int              // generazione del re-list in volo: solo il suo risultato libera refreshing (Piano 7, fix review 5)
	dirty         bool             // cambio arrivato durante il refresh: follow-up al risultato (Task 8)
	dirtyDeep     bool             // il follow-up richiesto durante il refresh è a profondità piena (Piano 7, Task 3)
	exhausted     bool             // ultima pagina ricevuta corta: nessuna pagina successiva (Piano 7, Task 3)
	loadingMore   bool             // pagina successiva in volo: mai due richieste insieme (Piano 7, Task 3)
	folder        mail.Mailbox     // cartella/label mostrata: zero value = posta in arrivo (Piano 7, Task 4)
	folderOpen    bool             // selettore cartelle aperto (`m`): i tasti vanno alla lista (Piano 7, Task 4)
	folderList    list.Model       // elenco del selettore cartelle (bubbles/list, folders.go)
	trashOpen     bool             // conferma "spostare nel cestino?" aperta (Piano 7, Task 5)
	trashRef      msg.MessageRef   // messaggio in conferma: letto all'apertura, non al sì

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
		store:      store,
		sender:     sender,
		styles:     st,
		spinner:    spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(st.Spinner)),
		help:       newHelp(st),
		inbox:      inbox.New(nil, st),
		folderList: newFolderList(st),
		search:     newSearchInput(st),
		label:      newLabelInput(st),
		cfgPath:    cfgPath,
		view:       viewInbox,
		th:         th,
		layout:     config.LayoutSingle,
		markRead:   config.PreviewMarkReadDelay,
		ctx:        ctx,
		cancel:     cancel,
		// Il Connect d'avvio emesso da Init è già in flight (bugfix
		// 2026-11-22): pending parte da 1 così la status bar mostra lo
		// spinner finché connResult non arriva — il load vero incrementa
		// di nuovo al dispatch. Le op successive idem (Task 6/7).
		pending: 1,
	}
}

// Init emette il Connect d'avvio (bugfix 2026-11-22: il contratto MailStore
// lo richiede prima di qualunque lettura): al connResult riuscito partono il
// load iniziale della lista (Query vuota, prima pagina da 50) e il feed Watch
// (Task 8) — al watchStarted la radice arma il loop waitForChange che guida
// il refresh automatico. In modalità wizard (Piano 4, Task 5) NIENTE parte:
// store/sender sono ancora il placeholder e la sessione vera nasce al
// completamento del wizard.
func (m Model) Init() tea.Cmd {
	if m.wizardActive {
		return m.wiz.Init()
	}
	return m.beginSession()
}

// beginSession è il flusso d'avvio normale (Init senza wizard, e ripartenza
// dopo il completamento del wizard): Connect PRIMA di qualunque lettura
// (bugfix 2026-11-22: con lo store IMAP reale load/Watch senza Connect
// falliscono con "non connesso"; il Mock la mascherava perché no-op),
// poi — al connResult — load della lista + feed Watch (connResult case).
func (m Model) beginSession() tea.Cmd {
	// Builder puro della cmd d'avvio (Init in v2 restituisce solo la cmd):
	// le mutazioni di stato (pending per il connect, reset connectFailed)
	// le fanno i call site in Update — lì il modello può essere restituito.
	return tea.Batch(connectCmd(m.store, m.ctx), m.spinner.Tick)
}

// Update gestisce i messaggi e tiene l'anteprima della vista a due colonne
// allineata al cursore (syncPreview, dopo ogni messaggio).
func (m Model) Update(ev tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(ev)
	nm, sync := next.(Model).syncPreview()
	if sync == nil {
		return nm, cmd
	}
	return nm, tea.Batch(cmd, sync)
}

// update è il routing dei messaggi: risultati delle Cmd, tick dello spinner,
// tasti di routing e resize del terminale.
func (m Model) update(ev tea.Msg) (tea.Model, tea.Cmd) {
	switch ev := ev.(type) {
	case statusMsg:
		// Timeout logico: lo status resta finché il prossimo evento non lo
		// sostituisce; nessun timer.
		m.status = ev
		return m, nil

	case connResult:
		// Esito del Connect d'avvio (bugfix 2026-11-22). Fallito: status
		// d'errore e NESSUN load/Watch (le letture fallirebbero comunque);
		// `R` rilancia beginSession. Riuscito: parte il vecchio flusso
		// (load iniziale + spinner + feed Watch).
		m.doneOp()
		if ev.err != nil {
			m.connectFailed = true
			m.status = statusMsg{text: fmt.Sprintf("connessione fallita: %v — premi R per riprovare", ev.err), isErr: true}
			return m, nil
		}
		m.connectFailed = false
		m.pending++
		m.listGen++
		return m, tea.Batch(loadRefs(m.store, m.folder.Name, m.listGen), m.spinner.Tick, startWatch(m.store, m.ctx))

	case msgsLoaded:
		m.doneOp()
		// Il token del re-list silenzioso si libera SOLO al risultato del
		// refresh in volo (fix review 5): un load non silenzioso o un risultato
		// stante non riapre il coalescing di un refresh ancora in corso. Il
		// risultato di quel refresh libera il token a QUALSIASI esito (un refresh
		// fallito non deve bloccare i successivi); dirty NON viene consumato: il
		// follow-up partirà alla prima riuscita (self-healing).
		freed := ev.silent && ev.gen == m.refreshGen
		if freed {
			m.refreshing = false
		}
		if ev.gen != m.listGen || ev.mailbox != m.folder.Name || ev.query != m.queryRaw {
			// Stante (Piano 7, Task 4, fix review 2): il risultato è di un'altra
			// generazione di lista (cambio cartella/ricerca/connect durante il
			// load, anche tornando sulla stessa cartella) o di un'altra cartella
			// o query: la lista visibile non va sovrascritta.
			// Piano 7, fix code-review 2: il refresh stante ha appena liberato il
			// token; un cambio arrivato mentre era in volo (dirty) non deve perdersi,
			// quindi il follow-up parte qui (dirtyDeep è consumato da requestRefresh).
			var follow tea.Cmd
			if freed && m.dirty {
				follow = m.requestRefresh(m.dirtyDeep)
			}
			return m, follow
		}
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
		// Paginazione (Piano 7, Task 3). limit 0 = risultato non paginato
		// (ricerca, risultato costruito a mano): nessuna pagina successiva.
		// Refresh silenzioso a pagina piena: la coda già caricata oltre la
		// pagina si conserva (Star/MarkRead rilistano solo 50, D5) — l'esito
		// esaurito resta quello di prima. Altrimenti esaurita = pagina corta.
		refs := ev.refs
		switch {
		case ev.limit == 0:
			m.exhausted = true
		case ev.silent && len(refs) == ev.limit && len(m.refs) > len(refs):
			refs = appendUnique(refs, m.refs[len(refs):])
		default:
			m.exhausted = len(refs) < ev.limit
		}
		if !ev.silent {
			// Lista sostituita: un'eventuale pagina in volo appartiene alla
			// lista precedente e non blocca più lo scroll della nuova.
			m.loadingMore = false
		}
		m.refs = refs
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
		cmd := m.inbox.SetRefs(refs)
		if ev.silent && len(refs) > 0 {
			// Ripristino del cursore dopo il refresh di un'op: stesso
			// messaggio se c'è ancora nell'elenco (anche se si è spostato),
			// altrimenti lo stesso indice clampato alla nuova lunghezza (il
			// messaggio operato può essere sparito, es. archiviato).
			idx := -1
			if hadSel {
				for i, r := range refs {
					if r.ID == selID {
						idx = i
						break
					}
				}
			}
			if idx < 0 {
				idx = min(oldIdx, len(refs)-1)
			}
			m.inbox.Select(idx)
		}
		// Coalescing (Task 8): se un cambio è arrivato mentre il refresh era
		// in volo, il follow-up parte ora (lo snapshot appena consumato è
		// comunque più vecchio del cambio).
		var follow tea.Cmd
		if m.dirty {
			follow = m.requestRefresh(false)
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
		m.relayout()
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

	case mailboxesLoaded:
		// Risposta di ListMailboxes per il selettore (Piano 7, Task 4). Chiuso
		// il selettore prima della risposta, il risultato è ignorato; errore →
		// status d'errore e nessuna modale; altrimenti la lista si riempie con
		// il cursore sulla cartella corrente.
		m.doneOp()
		if !m.folderOpen {
			return m, nil
		}
		if ev.err != nil {
			m.folderOpen = false
			m.status = statusMsg{text: fmt.Sprintf("cartelle: %v", ev.err), isErr: true}
			return m, nil
		}
		items := make([]list.Item, len(ev.boxes))
		cur := 0
		for i, b := range ev.boxes {
			items[i] = folderItem{box: b}
			if canonicalFolder(b) == m.folder {
				cur = i
			}
		}
		cmd := m.folderList.SetItems(items)
		m.folderList.Select(cur)
		return m, cmd

	case moreLoaded:
		// Pagina successiva (scroll in fondo, Piano 7, Task 3). Stante se la
		// ricerca è cambiata dopo la richiesta: la pagina non appartiene più
		// alla lista visibile e si scarta (il loadingMore lo libera il
		// msgsLoaded della lista nuova).
		m.doneOp()
		if ev.gen != m.listGen || ev.mailbox != m.folder.Name || ev.query != m.queryRaw {
			// Stante (fix review 2): pagina di un'altra generazione di lista
			// (anche tornando sulla stessa cartella con lo stesso nome).
			return m, nil
		}
		m.loadingMore = false
		if ev.err != nil {
			m.status = statusMsg{text: fmt.Sprintf("caricamento messaggi: %v", ev.err), isErr: true}
			return m, nil
		}
		m.exhausted = len(ev.refs) < pageSize
		m.refs = appendUnique(m.refs, ev.refs)
		// Piano 7, fix code-review 3: m.inbox si muta nella chiamata (ricevitore
		// puntatore): la cmd va calcolata PRIMA di costruire il Model restituito.
		cmd := m.inbox.AppendRefs(ev.refs)
		return m, cmd

	case opDone:
		m.doneOp()
		if ev.err == nil && len(ev.gone) > 0 {
			// Piano 7, fix review 1: gli ID usciti dalla cartella (Archive/Trash)
			// si tolgono dalla coda PRIMA del merge del refresh profondo, che
			// riaccoda le righe oltre refreshCap dalla lista precedente.
			m.refs = withoutIDs(m.refs, ev.gone)
		}
		if ev.send {
			// Invio concluso (ok o errore): riabilita un eventuale retry
			// (ctrl+s dopo un errore riapre la conferma).
			m.sending = false
		}
		if ev.err != nil {
			m.status = statusMsg{text: fmt.Sprintf("%s: %v", ev.label, ev.err), isErr: true}
			return m, nil
		}
		m.status = statusMsg{text: ev.label, isOK: true}
		// Piano 7.2, fix review: il segna-come-letto si vede subito in lista.
		localCmd := m.setLocalRead(ev.localID, ev.localRead)
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
			// risultato (dirty). deep (Piano 7, D5): profondità piena solo per
			// Archive/Trash, che cambiano la composizione della lista.
			// Piano 7, fix code-review 3: requestRefresh muta m (ricevitore puntatore):
			// va chiamato prima di restituire m, mai nella stessa return.
			follow := m.requestRefresh(ev.deep)
			return m, tea.Batch(localCmd, follow)
		}
		return m, localCmd

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
		// re-list, la lista conserva ref, cursore e stato. Con due colonne il
		// thread torna alla colonna dell'anteprima (Piano 7.2, Task 4).
		m.view = viewInbox
		m.relayout()
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
		m.status = statusMsg{text: "aperto nel browser", isOK: true}
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
		m.status = statusMsg{text: "salvato in " + ev.path, isOK: true}
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
		m.th = ev.Th
		m.applyStyles(ev.St)
		m.status = statusMsg{text: fmt.Sprintf("tema %s applicato", ev.Th.Name), isOK: true}
		return m, saveThemeCmd(m.cfgPath, ev.Th.Name)

	case themePersisted:
		// Persistenza del tema riuscita: il feedback "tema … applicato" resta.
		// Fallita: solo status d'errore, il tema runtime non si perde.
		if ev.err != nil {
			m.status = statusMsg{text: fmt.Sprintf("salvataggio tema: %v", ev.err), isErr: true}
		}
		return m, nil

	case settingsSaved:
		// Piano 7.2, Task 3: salvataggio del menu `,`. Fallito → status d'errore;
		// i valori restano applicati a runtime (come per il tema).
		if ev.err != nil {
			m.status = statusMsg{text: fmt.Sprintf("salvataggio impostazioni: %v", ev.err), isErr: true}
			return m, nil
		}
		m.status = statusMsg{text: "impostazioni salvate", isOK: true}
		return m, nil

	case previewDue:
		// Piano 7.2, Task 4: cursore fermo da 150 ms → fetch dell'anteprima, salvo
		// token stantio o messaggio già caricato (A → B → A non rifetcha).
		if ev.tok != m.prevTok || !m.previewVisible() {
			return m, nil
		}
		if m.previewReady() {
			// Piano 7.2, fix review: anteprima già caricata (ritorno sullo stesso
			// messaggio entro 150 ms): nessun fetch, ma il segna-come-letto va
			// riarmato, altrimenti il messaggio resta non letto.
			return m.previewMarkRead(ev.tok)
		}
		m.pending++
		return m, tea.Batch(fetchPreview(m.store, ev.tok, m.prevRef.ID), m.spinner.Tick)

	case previewLoaded:
		// Esito del fetch dell'anteprima: scartato se il token non è più quello
		// corrente o il messaggio non è più sotto il cursore (il doneOp chiude
		// comunque l'op in volo).
		m.doneOp()
		if ev.tok != m.prevTok || !m.previewVisible() || ev.id != m.prevRef.ID {
			return m, nil
		}
		if ev.err != nil {
			m.status = statusMsg{text: fmt.Sprintf("anteprima: %v", ev.err), isErr: true}
			return m, nil
		}
		if ev.m == nil {
			m.status = statusMsg{text: "anteprima: risposta vuota dallo store", isErr: true}
			return m, nil
		}
		m.thread = thread.New(m.prevRef, ev.m, m.styles)
		m.relayout()
		return m.previewMarkRead(ev.tok)

	case previewReadDue:
		// Segna come letto ritardato (U5, "dopo 2 secondi"): solo se il messaggio
		// è ancora quello in anteprima, con lo stesso token e ancora non letto.
		if ev.tok != m.prevTok || !m.previewVisible() || !m.previewReady() || m.thread.Ref().ID != ev.id || !m.thread.Ref().Unread {
			return m, nil
		}
		m.thread.MarkRead()
		return m, markRead(m.store, ev.id, true, false)

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
		// risultati di fetch stanti — anche col modale discard aperto (ctrl+s
		// in volo + esc prima della consegna: confermarlo lascerebbe
		// confirmSend acceso oltre lo scarto, e `s` nell'inbox invierebbe la
		// bozza scartata). Con un invio già in flight (sending) è scartato
		// SILENZIOSAMENTE: riaprire la conferma permetterebbe un secondo
		// sendCmd sulla stessa bozza (doppia email) — lo spinner in status
		// bar segnala già che l'invio è in corso. Il segnale è `sending`
		// (invio specifico), NON pending: op estranee in flight (load/re-list
		// iniziale, refresh) non devono bloccare il primo invio.
		if m.view != viewComposer || m.sending || m.discardOpen {
			return m, nil
		}
		// Piano 7, Task 6 + fix review 7: un destinatario malformato o assente
		// non apre la conferma (NormalizeRecipients: formato di A/Cc/Ccn e almeno
		// un indirizzo). Il controllo viene DOPO i guard sopra: una richiesta
		// stante o in volo resta scartata in silenzio, senza errori in status. Il
		// composer resta aperto con la bozza intatta.
		// Piano 7.1, Task 3: la bozza confermata è quella normalizzata (dominio in
		// punycode, nome RFC 2047): conferma e Sender vedono la stessa forma.
		norm, err := msg.NormalizeRecipients(ev.Out)
		if err != nil {
			m.status = statusMsg{text: err.Error(), isErr: true}
			return m, nil
		}
		m.draft = norm
		m.confirmSend = true
		return m, nil

	case spinner.TickMsg:
		if m.wizardActive {
			// In modalità wizard lo spinner attivo è quello DEL WIZARD (passi
			// testing/saving): i tick vanno a lui, non al spinner del radice.
			var cmd tea.Cmd
			m.wiz, cmd = m.wiz.Update(ev)
			return m, cmd
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(ev)
		if m.pending <= 0 {
			return m, nil // niente op in flight: spinner fermo, non ri-armare
		}
		return m, cmd

	case tea.WindowSizeMsg:
		m.width, m.height = ev.Width, ev.Height
		// Il corpo occupa tutte le righe tranne l'ultima (status bar). Con due
		// colonne (Piano 7.2, Task 4) lista e anteprima prendono la loro quota.
		m.relayout()
		m.folderList.SetSize(ev.Width, max(0, m.height-2)) // -2: titolo del selettore + status bar
		if m.wizardActive {
			// Gli input del wizard si adattano alla larghezza (Piano 4, Task 5).
			m.wiz.SetWidth(ev.Width)
		}
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
			// overlay, wizard): quitCmd cancella il ctx del feed e esce. Il
			// routing resta identico a Task 3-8 per tutti gli altri tasti
			// (keymap centrale in keys.go, Task 9).
			return m, m.quitCmd()
		}
		if m.wizardActive {
			// Wizard first-run (Piano 4, Task 5): TUTTI i tasti vanno al
			// wizard — enter avanza, esc torna al passo precedente, `s` salta
			// in demo, il resto è testo degli input. I tasti operativi
			// dell'app (j/k/r/a/…, q incluso) non significano nulla qui: la
			// casella non esiste ancora.
			var cmd tea.Cmd
			m.wiz, cmd = m.wiz.Update(ev)
			return m, cmd
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
		if m.menu.open {
			// Menu impostazioni (Piano 7.2, Task 3): tutti i tasti vanno al menu;
			// ctrl+c è già uscito sopra, nessun tasto operativo scatta sotto.
			return m.settingsKey(ev)
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
		if m.folderOpen {
			// Selettore cartelle aperto (Piano 7, Task 4): invio apre la cartella
			// sotto il cursore, esc chiude, il resto naviga la lista (j/k, frecce,
			// pagine). Le op della lista dietro non scattano (come col prompt).
			switch ev.String() {
			case "enter":
				return m.chooseFolder()
			case "esc":
				m.folderOpen = false
				return m, nil
			default:
				var cmd tea.Cmd
				m.folderList, cmd = m.folderList.Update(ev)
				return m, cmd
			}
		}
		if m.trashOpen {
			// Conferma cestino (Piano 7, Task 5, D4): s sposta il messaggio
			// catturato all'apertura, esc annulla — gli altri tasti sono ignorati
			// (nessuna op dietro la conferma, come per il discard).
			switch ev.String() {
			case "s":
				return m.confirmTrash()
			case "esc":
				m.trashOpen = false
			}
			return m, nil
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
				if m.sending {
					// Invio già in flight: rifutare — un secondo sendCmd sulla
					// stessa bozza sarebbe una doppia email. Stato non
					// raggiungibile via UI (il SendRequestedMsg è già
					// guardato): difesa in profondità, la conferma resta
					// aperta (esc può chiuderla).
					return m, nil
				}
				m.confirmSend = false
				m.sending = true // blocca un secondo invio finché opDone
				m.pending++      // invio in flight: spinner finché opDone
				return m, tea.Batch(sendCmd(m.sender, m.draft), m.spinner.Tick)
			case "esc":
				m.confirmSend = false // back al composer: la bozza resta
			}
			return m, nil
		}
		if m.threadTarget() && m.thread.ModalOpen() {
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
		if m.previewFocused() {
			// Piano 7.2, Task 4 (U6): con il focus sull'anteprima esc torna alla
			// lista (non esce dall'app) e invio apre a schermo intero il messaggio
			// in anteprima. Il resto segue il routing sotto.
			switch ev.String() {
			case "esc", "q":
				// Piano 7.2, fix review: q torna alla lista come esc (non esce dall'app).
				m.focusPreview = false
				return m, nil
			case "enter":
				ref, ok := m.previewRef()
				if !ok {
					return m, nil
				}
				return m, func() tea.Msg { return inbox.SelectedMsg{Ref: ref} }
			}
		}
		// Il routing usa la keymap centrale (Task 9): stesse chiavi e stesso
		// ordine del vecchio switch su ev.String() (a/s/v/l → q/esc → …).
		switch {
		case m.connectFailed && key.Matches(ev, keys.Connect):
			// Bugfix 2026-11-22: Connect d'avvio fallito → R riprova la
			// sequenza completa (connect → load → watch). Nel routing
			// normale: se un input è attivo (ricerca, composer, prompt) R è
			// testo del campo, non un comando. Il flag si spegne SUBITO:
			// un secondo R durante il retry non lancia Connect concorrenti.
			m.pending++ // il connect è un'op: spinner finché connResult
			m.connectFailed = false
			return m, m.beginSession()
		case key.Matches(ev, keys.Reply, keys.Forward):
			// r/f (Piano 4, Task 3): nell'inbox fetch-then-compose (cache-
			// first via store.FetchMessage), nel thread composer immediato
			// dal messaggio già caricato. Il composer precompilato sostituisce
			// la vista corrente (il back del composer torna lì).
			return m.startCompose(ev.String() == "f")
		case key.Matches(ev, keys.Focus):
			// Tab (Piano 7.2, Task 4, U6): alterna il focus lista/anteprima; senza
			// due colonne non fa nulla.
			if m.splitOn() && m.view == viewInbox {
				m.focusPreview = !m.focusPreview
			}
			return m, nil
		case key.Matches(ev, keys.Trash):
			// # (Piano 7, Task 5): apre la conferma sul messaggio corrente (lista
			// o thread). Nel cestino la conferma non si apre (askTrash).
			return m.askTrash()
		case key.Matches(ev, keys.Folders):
			// Selettore cartelle e label (Piano 7, Task 4): solo dalla lista; nel
			// thread `m` cade nella delega di coda senza effetti.
			if m.view == viewInbox {
				return m.openFolders()
			}
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
		case key.Matches(ev, keys.Settings):
			// Menu impostazioni (Piano 7.2, Task 3): da inbox e thread.
			if m.view == viewInbox || m.view == viewThread {
				return m.openSettings()
			}
		case key.Matches(ev, keys.Help):
			// Overlay help completo (Task 9): si apre sopra il corpo, ?/esc
			// lo chiudono (gestiti in testa al case KeyPressMsg).
			m.showHelp = true
			return m, nil
		case key.Matches(ev, keys.Search):
			if m.view == viewInbox {
				m.view = viewSearch
				// Focus del textinput: la cmd avvia il blink del cursore.
				// Piano 7, fix code-review 3: Focus muta m.search (ricevitore puntatore).
				follow := m.search.Focus()
				return m, follow
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
			// — cmdmode.go). Con il focus sull'anteprima (Piano 7.2, Task 4) vale
			// lo stesso. Nelle altre viste non significa nulla: la key cade nella
			// delega di coda senza effetti.
			if m.threadTarget() {
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
			if m.previewFocused() {
				// Focus sull'anteprima (Piano 7.2, Task 4): j/k/pgup/pgdn/space/d/u
				// scorrono il messaggio; la lista non si muove.
				if !m.previewReady() {
					return m, nil
				}
				var cmd tea.Cmd
				m.thread, cmd = m.thread.Update(ev)
				return m, cmd
			}
			var cmd tea.Cmd
			m.inbox, cmd = m.inbox.Update(ev)
			return m.loadMoreAtEnd(cmd)
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
		// conferme discard/invio (P4 Task 4) è ignorato come i tasti; in
		// modalità wizard (P4 Task 5) il setup è keyboard-first (i click non
		// selezionano nulla: la lista non esiste ancora).
		if m.showHelp || m.labelOpen || m.folderOpen || m.trashOpen || m.discardOpen || m.confirmSend || m.wizardActive || m.menu.open || m.thread.ModalOpen() {
			return m, nil
		}
		if m.view == viewInbox {
			// Due colonne (Piano 7.2, Task 4): la colonna destra è l'anteprima. La
			// rotella la scorre (a prescindere dal focus); i click su di essa non
			// fanno nulla, e il separatore conta come anteprima.
			if m.splitOn() {
				if list, _ := splitWidths(m.width); mouseX(ev) >= list {
					// Piano 7.2, fix review: click sinistro sull'anteprima = focus; doppio
					// click = thread a schermo intero (previewClick).
					if click, ok := ev.(tea.MouseClickMsg); ok && click.Button == tea.MouseLeft {
						return m.previewClick()
					}
					if _, wheel := ev.(tea.MouseWheelMsg); wheel && m.previewReady() {
						var cmd tea.Cmd
						m.thread, cmd = m.thread.Update(ev)
						return m, cmd
					}
					return m, nil
				}
			}
			var cmd tea.Cmd
			m.inbox, cmd = m.inbox.Update(ev)
			return m.loadMoreAtEnd(cmd)
		}
		if m.view == viewThread {
			var cmd tea.Cmd
			m.thread, cmd = m.thread.Update(ev)
			return m, cmd
		}
		return m, nil

	case wizard.CompletedMsg:
		// Completamento del first-run wizard (Piano 4, Task 5): skip → demo
		// col seed, successo → factory dei backend reali; poi il flusso
		// normale parte (load + feed). Stanti scartati dal handler.
		return m.wizardCompleted(ev)

	default:
		// In modalità wizard i messaggi non-tasto (blink dei cursori degli
		// input, ecc.) raggiungono il wizard: senza, i cursori resterebbero fermi.
		if m.wizardActive {
			var cmd tea.Cmd
			m.wiz, cmd = m.wiz.Update(ev)
			return m, cmd
		}
		// Menu impostazioni in modifica (Piano 7.2, Task 3): blink del cursore del
		// campo Nome/Firma.
		if m.menu.open && m.menu.editing {
			var cmd tea.Cmd
			if m.menu.cur == rowName {
				m.menu.name, cmd = m.menu.name.Update(ev)
			} else {
				m.menu.sig, cmd = m.menu.sig.Update(ev)
			}
			return m, cmd
		}
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

// applyStyles applica gli stili di un tema alla radice e alle sub-viste
// (spinner, help, inbox, selettore cartelle, thread) senza perdere lo stato.
// Usato dal cambio tema `:theme` e dall'anteprima del menu impostazioni
// (Piano 7.2, Task 3), che al ripristino riapplica il tema di apertura.
func (m *Model) applyStyles(st styles.Styles) {
	m.styles = st
	m.spinner.Style = st.Spinner
	m.help = newHelp(st)
	m.inbox.SetStyles(st)
	m.folderList.SetDelegate(folderDelegate{st: st})
	m.thread.SetStyles(st)
	if m.menu.open {
		m.menu.name.SetStyles(settingsInputStyles(st))
		m.menu.sig.SetStyles(settingsTextareaStyles(st))
	}
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
	m.listGen++ // nuova generazione: i risultati della lista precedente sono stanti (fix review 2)
	return m, tea.Batch(submitQuery(m.store, raw, m.folder.Name, m.listGen), m.spinner.Tick)
}

// loadRefs esegue ListMessages fuori dal loop UI. Il contesto è background
// finché il wiring del programma non esponga il ctx del programma (Task 7/8).
func loadRefs(st mail.MailStore, box string, gen int) tea.Cmd {
	return func() tea.Msg {
		refs, err := st.ListMessages(context.Background(), msg.Query{Label: box}, mail.Page{Limit: pageSize})
		return msgsLoaded{refs: refs, err: err, limit: pageSize, mailbox: box, gen: gen}
	}
}

// loadMoreAtEnd chiede la pagina successiva quando il cursore è in fondo alla
// lista (Piano 7, Task 3): scroll con tasti o rotella nella vista inbox. Non
// parte con una ricerca attiva (non paginata), con la lista già esaurita, o con
// una pagina in volo (mai due richieste insieme). L'offset è il numero di righe
// caricate: il dedup di moreLoaded assorbe gli slittamenti.
func (m Model) loadMoreAtEnd(cmd tea.Cmd) (Model, tea.Cmd) {
	if m.queryRaw != "" || m.exhausted || m.loadingMore || !m.inbox.AtEnd() {
		return m, cmd
	}
	m.loadingMore = true
	m.pending++
	return m, tea.Batch(cmd, loadMore(m.store, m.folder.Name, len(m.refs), m.queryRaw, m.listGen), m.spinner.Tick)
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
	if m.menu.open {
		return m.settingsView()
	}
	if m.folderOpen {
		return m.foldersView()
	}
	if m.view == viewInbox && m.splitOn() {
		return m.splitView()
	}
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
	case viewWizard:
		return m.wizardBody()
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
	if m.trashOpen {
		// Conferma cestino (Piano 7, Task 5, D4): stile StatusErr come il discard.
		return m.renderBar(m.styles.StatusErr, "spostare nel cestino? s conferma · esc annulla")
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
	if m.folder.Name != "" {
		// Fuori dalla posta in arrivo la status bar dice in quale cartella si è
		// (Piano 7, Task 4). Senza altro testo il tag resta da solo (fix review 6:
		// il nome non è ripetuto come messaggio).
		tag := "[" + folderName(m.folder) + "]"
		if text == "" {
			text = tag
		} else {
			text = tag + " " + text
		}
	}
	if m.pending > 0 {
		text = m.spinner.View() + " " + text
	}
	if m.status.isErr {
		return m.renderBar(m.styles.StatusErr, text)
	}
	if m.status.isOK {
		// Feedback positivo (Piano 3 polish, use-or-drop — adottato al Piano 4
		// Task 5): "inviato", "salvato", "tema applicato", setup completato.
		return m.renderBar(m.styles.StatusOK, text)
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
