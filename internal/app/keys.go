// Keymap centrale + help (Task 9): i binding sono definiti UNA volta qui
// (bubbles/key) e consumati da due lati — il routing di Update (key.Matches)
// e il help (barra breve sempre visibile nella status bar, overlay completo
// via `?`). Il flusso di Update NON cambia rispetto a Task 3-8: le stesse
// chiavi, nello stesso ordine, con le stesse gate di vista.
//
// Per vista il help espone binding diverse (shortHelpFor/fullHelpFor): inbox,
// thread, ricerca e prompt label; il prompt label non ha barra help visibile
// (il prompt rende sé stesso nella status bar) ma le sue binding esistono e
// sono documentate come le altre.
package app

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/ui/styles"
)

// appKeys è la keymap dell'app: una definizione per azione, con i tasti
// alternativi nella stessa binding. Le descrizioni sono in italiano, coerenti
// con la UI. Il campo Help() è l'unica fonte del help generato.
type appKeys struct {
	Move       key.Binding // j k ↑ ↓ — cursore lista / scroll thread
	Scroll     key.Binding // pgup pgdn space d u — scroll pagina viewport (thread; f/b liberati dal rebind, P4 Task 3)
	OpenSel    key.Binding // invio — apri il messaggio selezionato (inbox)
	Quit       key.Binding // q esc — esci dall'app (inbox)
	Back       key.Binding // esc q — torna alla lista (thread)
	ForceQuit  key.Binding // ctrl+c — uscita immediata da qualunque stato
	Connect    key.Binding // R — riprova la connessione d'avvio (solo dopo un fallimento; bugfix 2026-11-22)
	Search     key.Binding // / — cerca
	Reply      key.Binding // r — rispondi (inbox: ref selezionato, thread: msg caricato; P4 Task 3)
	Forward    key.Binding // f — inoltra (idem; f liberato dal rebind scroll, P4 Task 3)
	Composer   key.Binding // c — scrivi: nuovo messaggio (P4 Task 3)
	Archive    key.Binding // a — archivia
	Star       key.Binding // s — stella on/off
	Read       key.Binding // v — letta/non letta
	Label      key.Binding // l — etichetta
	Folders    key.Binding // m — cartelle e label (selettore, Piano 7, Task 4)
	Pick       key.Binding // invio — apri la cartella scelta (selettore, Piano 7, Task 4)
	Trash      key.Binding // # — sposta nel cestino (apre la conferma, Piano 7, Task 5)
	TrashOK    key.Binding // s — conferma lo spostamento nel cestino (Piano 7, Task 5)
	Open       key.Binding // o — apri nel browser (thread)
	Command    key.Binding // : — command line :save/:open/:theme (thread)
	Submit     key.Binding // invio — esegui la ricerca
	Cancel     key.Binding // esc — annulla (ricerca, prompt label, composer)
	Apply      key.Binding // invio — applica l'etichetta
	WizardCont key.Binding // invio — continua al passo successivo (wizard)
	WizardSkip key.Binding // s — salta il setup ed entra in demo (wizard)
	Help       key.Binding // ? — aiuto completo
	Settings   key.Binding // , — menu impostazioni (Piano 7.2, Task 3)
	SetValue   key.Binding // ←→ — cambia il valore della voce (menu impostazioni)
	SetEdit    key.Binding // invio — modifica Nome/Firma (menu impostazioni)
	SetSave    key.Binding // invio — salva e chiude (menu impostazioni, voce a scelta)
	SetDone    key.Binding // esc — torna alla lista delle voci (campo in modifica)
	Focus      key.Binding // tab — sposta il focus fra lista e anteprima (Piano 7.2, Task 4)
}

// keys è LA keymap centrale dell'app: condivisa da Update e dal help. Se un
// tasto cambia qui, il test TestRoutedKeysAreDocumented ricorda di tenere
// allineato il routing (e viceversa).
var keys = appKeys{
	Move:       key.NewBinding(key.WithKeys("j", "k", "up", "down"), key.WithHelp("j/k", "sposta")),
	Scroll:     key.NewBinding(key.WithKeys("pgup", "pgdn", "space", "d", "u"), key.WithHelp("pgup/pgdn/space/d/u", "scroll pagina")),
	OpenSel:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("invio", "apri")),
	Quit:       key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q/esc", "esci")),
	Back:       key.NewBinding(key.WithKeys("esc", "q"), key.WithHelp("esc/q", "torna alla lista")),
	ForceQuit:  key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "esci subito")),
	Connect:    key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "riprova connessione")),
	Search:     key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "cerca")),
	Reply:      key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rispondi")),
	Forward:    key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "inoltra")),
	Composer:   key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "scrivi")),
	Archive:    key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "archivia")),
	Star:       key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "stella")),
	Read:       key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "letta/non letta")),
	Label:      key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "etichetta")),
	Folders:    key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "cartelle")),
	Pick:       key.NewBinding(key.WithKeys("enter"), key.WithHelp("invio", "apri cartella")),
	Trash:      key.NewBinding(key.WithKeys("#"), key.WithHelp("#", "cestino")),
	TrashOK:    key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "conferma cestino")),
	Open:       key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "apri nel browser")),
	Command:    key.NewBinding(key.WithKeys(":"), key.WithHelp(":", "comandi")),
	Submit:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("invio", "cerca")),
	Cancel:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "annulla")),
	Apply:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("invio", "applica etichetta")),
	WizardCont: key.NewBinding(key.WithKeys("enter"), key.WithHelp("invio", "continua")),
	WizardSkip: key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "salta (demo)")),
	Help:       key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "aiuto")),
	Settings:   key.NewBinding(key.WithKeys(","), key.WithHelp(",", "impostazioni")),
	SetValue:   key.NewBinding(key.WithKeys("left", "right", "h", "l"), key.WithHelp("←→", "valore")),
	SetEdit:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("invio", "modifica")),
	SetSave:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("invio", "salva")),
	SetDone:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "torna alla lista")),
	Focus:      key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "lista/anteprima")),
}

// newHelp costruisce il modello help (bubbles/help) con gli stili del tema:
// i default di bubbles hardcodano hex, e il vincolo "solo slot del tema"
// vale anche qui. Chiavi in Accent (slot SearchInput), descrizioni e
// separatori attenuati (slot ListDate/Muted).
func newHelp(st styles.Styles) help.Model {
	h := help.New()
	h.Styles = help.Styles{
		Ellipsis:       st.ListDate,
		ShortKey:       st.SearchInput,
		ShortDesc:      st.ListDate,
		ShortSeparator: st.ListDate,
		FullKey:        st.SearchInput,
		FullDesc:       st.ListDate,
		FullSeparator:  st.ListDate,
	}
	return h
}

// shortHelpFor è l'help breve per lo stato dato (vista + prompt label): la
// status bar lo rende sempre visibile, accanto all'eventuale testo di status.
// La lunghezza è trattata dal truncamento di bubbles/help (SetWidth).
func (m Model) shortHelpFor(labelOpen bool, v view) []key.Binding {
	if m.menu.open {
		// Menu impostazioni (Piano 7.2, Task 3): in modifica resta solo il ritorno
		// alla lista; altrimenti navigazione, valore, invio e annulla.
		if m.menu.editing {
			return []key.Binding{keys.SetDone}
		}
		return []key.Binding{keys.Move, keys.SetValue, keys.SetEdit, keys.SetSave, keys.Cancel}
	}
	if labelOpen {
		// Prompt label: invio applica, esc annulla (non mostrato visivamente:
		// il prompt occupa la status bar, resta la documentazione centrale).
		return []key.Binding{keys.Apply, keys.Cancel}
	}
	if m.folderOpen {
		// Selettore cartelle (Piano 7, Task 4): invio apre, esc chiude.
		return []key.Binding{keys.Pick, keys.Cancel}
	}
	if m.trashOpen {
		// Conferma cestino (Piano 7, Task 5): s sposta, esc annulla.
		return []key.Binding{keys.TrashOK, keys.Cancel}
	}
	switch v {
	case viewSearch:
		return []key.Binding{keys.Submit, keys.Cancel}
	case viewWizard:
		// Wizard first-run (Piano 4, Task 5): i passi mostrano il loro hint
		// dedicato nel corpo; la barra documenta invio/s e l'uscita. `?` NON è
		// publicizzato: i tasti vanno tutti al wizard (precedente composer).
		return []key.Binding{keys.WizardCont, keys.WizardSkip, keys.ForceQuit}
	case viewThread:
		return []key.Binding{keys.Back, keys.Reply, keys.Forward, keys.Archive, keys.Trash, keys.Star, keys.Read, keys.Label, keys.Open, keys.Command, keys.Settings, keys.Help}
	case viewComposer:
		// Nel composer i tasti operativi sono nella riga d'uso del modello
		// (tab/ctrl+e/ctrl+s) e la barra documenta solo annulla: `?` qui è
		// testo nel campo focalizzato (tutti i tasti vanno al composer),
		// pubblicizzarlo sarebbe mentire — come nella vista ricerca.
		return []key.Binding{keys.Cancel}
	default: // inbox
		if m.connectFailed {
			// Connect d'avvio fallito (bugfix 2026-11-22): la sola azione
			// utile è riprovare; le letture fallirebbero comunque. Quit è
			// pubblicizzato perché funziona: l'uscita pulita resta possibile.
			return []key.Binding{keys.Connect, keys.Quit, keys.ForceQuit}
		}
		return []key.Binding{keys.Quit, keys.Search, keys.Reply, keys.Forward, keys.Composer, keys.Archive, keys.Trash, keys.Star, keys.Read, keys.Label, keys.Folders, keys.Settings, keys.Help}
	}
}

// fullHelpFor è l'help completo per lo stato dato: gruppi (colonne) per
// categoria. L'overlay `?` lo rende con bubbles/help (FullHelpView).
func (m Model) fullHelpFor(labelOpen bool, v view) [][]key.Binding {
	if m.menu.open {
		return [][]key.Binding{
			{keys.Move, keys.SetValue},
			{keys.SetEdit, keys.SetSave, keys.SetDone, keys.Cancel},
		}
	}
	if labelOpen {
		return [][]key.Binding{{keys.Apply, keys.Cancel, keys.Help}}
	}
	if m.folderOpen {
		return [][]key.Binding{{keys.Pick, keys.Cancel}}
	}
	if m.trashOpen {
		return [][]key.Binding{{keys.TrashOK, keys.Cancel}}
	}
	switch v {
	case viewSearch:
		return [][]key.Binding{{keys.Submit, keys.Cancel}, {keys.Help}}
	case viewWizard:
		return [][]key.Binding{{keys.WizardCont, keys.WizardSkip, keys.ForceQuit}}
	case viewThread:
		return [][]key.Binding{
			{keys.Move, keys.Scroll, keys.Open, keys.Back},
			{keys.Reply, keys.Forward, keys.Archive, keys.Trash, keys.Star, keys.Read, keys.Label, keys.Command, keys.Settings},
			{keys.Help},
		}
	case viewComposer:
		return [][]key.Binding{{keys.Cancel, keys.Help}}
	default: // inbox
		return [][]key.Binding{
			{keys.Move, keys.OpenSel, keys.Search, keys.Reply, keys.Forward, keys.Composer, keys.Quit},
			{keys.Archive, keys.Trash, keys.Star, keys.Read, keys.Label, keys.Folders, keys.Settings},
			{keys.Help},
		}
	}
}

// ShortHelp implementa help.KeyMap: la barra breve della vista attiva.
func (m Model) ShortHelp() []key.Binding {
	return m.shortHelpFor(m.labelOpen, m.view)
}

// FullHelp implementa help.KeyMap: i gruppi dell'overlay per la vista attiva.
func (m Model) FullHelp() [][]key.Binding {
	return m.fullHelpFor(m.labelOpen, m.view)
}

// quitCmd è l'uscita pulita (mandato Task 8, implementato dal Task 9):
// cancella il ctx del feed Watch — il loop waitForChange si sblocca su
// ctx.Done e finisce — e poi esce dal programma. Il Msg è tea.QuitMsg: il
// runtime lo gestisce nell'event loop (l'Update non vede mai QuitMsg,
// verificato su bubbletea v2.1.2). LIMITAZIONE documentata (doc di Model): una
// SIGINT esterna arriva come tea.InterruptMsg e NON passa da Update — in quel
// percorso il ctx resta aperto fino all'uscita del processo.
func (m Model) quitCmd() tea.Cmd {
	return func() tea.Msg {
		m.cancel()
		return tea.QuitMsg{}
	}
}
