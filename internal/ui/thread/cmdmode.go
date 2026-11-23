// Modalità comando `:` nel thread (Piano 4, Task 2): `:` apre una command
// line inline sotto il corpo (textinput, stile SearchInput come i prompt di
// ricerca/label); mentre è aperta TUTTI i tasti sono testo (enter esegue, esc
// annulla — BackMsg non scatta). Comandi (parse semplice split-spaces; i
// percorsi con spazi non sono supportati, documentato nell'uso):
//
//	:save <n> [percorso]  — salva l'allegato n (1-based, elenco a fondo
//	                        messaggio). Senza percorso → filepicker bubbles
//	                        sulla directory corrente (invio conferma: su un
//	                        file salva lì, su una directory salva DENTRO con
//	                        il nome originale; esc annulla).
//	:open <n>             — salva in un tmpfile in os.TempDir col nome
//	                        originale e apre con open/xdg-open (stesso
//	                        meccanismo del tasto `o`).
//	:theme <nome>         — cambia tema a runtime (theme.Load + styles.New)
//	                        e notifica alla radice (ThemeChanged) per
//	                        ri-temizzare UI e persistere la config.
//	:theme (solo)         — elenca i temi disponibili (theme.Names).
//
// Il modello NON tocca il MailStore: produce messaggi di protocollo che la
// radice esegue fuori dal loop UI (SaveRequested → store.SaveAttachment).
package thread

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/filepicker"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/theme"
	"github.com/andsigno82/gmail-tui/internal/ui/styles"
)

// SaveRequested chiede alla radice il salvataggio di un allegato (n dell'
// elenco già mappato sul PartIndex filtrato). Open=true (da `:open`) chiede
// l'apertura con open/xdg-open DOPO il salvataggio riuscito (stesso meccanismo
// del tasto `o`: OpenCmd → OpenedMsg).
type SaveRequested struct {
	ID   msg.MessageID
	Part int
	Path string
	Open bool
}

// ThemeChanged notifica il cambio tema a runtime: la radice sostituisce gli
// stili (propri e delle sub-viste) e persiste il nome nella config.
// È definito QUI (lato emittente): messages.go vive nel package app, che
// importa già thread — un tipo lì referenziato dal thread creerebbe un ciclo.
type ThemeChanged struct {
	Th theme.Theme
	St styles.Styles
}

// CommandFeedback porta l'esito di un comando (o il suo errore) alla status
// bar della radice: elenco temi, errori d'uso, comando sconosciuto.
type CommandFeedback struct {
	Text  string
	IsErr bool
}

// Testi d'uso ed errori dei comandi (fonte dell'help e dei feedback).
const (
	saveUsage  = "uso: :save <n> [percorso]"
	openUsage  = "uso: :open <n>"
	themeUsage = "uso: :theme <nome>"
)

// commandList è l'elenco dei comandi mostrato per i comandi sconosciuti.
const commandList = "comandi: :save <n> [percorso], :open <n>, :theme <nome>"

// pickerHint è la riga di aiuto sotto il filepicker.
const pickerHint = "invio conferma · esc annulla"

// command è il risultato del parse della command line.
type command struct {
	name string
	args []string
}

// parseCommand splitta la riga su spazi; i due-punti iniziali sono opzionali
// (il prompt li mostra, il valore digitato di norma non li contiene). Righe
// vuote rendono un comando senza nome (chiusura silenziosa).
func parseCommand(raw string) command {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, ":")
	f := strings.Fields(raw)
	if len(f) == 0 {
		return command{}
	}
	if len(f) == 1 {
		return command{name: f[0]}
	}
	return command{name: f[0], args: f[1:]}
}

// newCommandInput costruisce il textinput della command line: prompt ": "
// nello slot SearchInput (come ricerca e label), placeholder con l'uso,
// cursore blocco lampeggiante. Nasce NON focalizzato: il focus arriva con `:`.
func newCommandInput(st styles.Styles) textinput.Model {
	ti := textinput.New()
	ti.Prompt = ": "
	ti.Placeholder = "save <n> [percorso] · open <n> · theme <nome>"
	ti.CharLimit = 256
	ti.SetStyles(commandInputStyles(st))
	return ti
}

// commandInputStyles mappa gli slot del tema sugli stili del textinput.
func commandInputStyles(st styles.Styles) textinput.Styles {
	var s textinput.Styles
	s.Focused.Prompt = st.SearchInput
	s.Focused.Placeholder = st.ListDate
	s.Blurred.Prompt = st.SearchInput
	s.Blurred.Placeholder = st.ListDate
	s.Cursor = textinput.CursorStyle{Shape: tea.CursorBlock, Blink: true}
	return s
}

// themedPickerStyles mappa gli slot del tema sugli stili del filepicker
// bubbles (i default hardcodano hex: vale il vincolo "solo slot del tema").
func themedPickerStyles(st styles.Styles) filepicker.Styles {
	return filepicker.Styles{
		DisabledCursor:   st.ListDate,
		Cursor:           st.SearchInput,
		Symlink:          st.ListDate,
		Directory:        st.ThreadHeader,
		File:             st.ListNormal,
		DisabledFile:     st.ListDate,
		Permission:       st.ListDate,
		Selected:         st.ListSelected,
		DisabledSelected: st.ListDate,
		FileSize:         st.ListDate,
		EmptyDirectory:   st.ListDate,
	}
}

// openCommand apre la command line: input svuotato e focalizzato, la viewport
// si riduce di una riga per fare posto (resize ricalcola l'altezza esatta).
func (m *Model) openCommand() tea.Cmd {
	m.cmdInput.Reset()
	m.cmdOpen = true
	m.resize()
	return m.cmdInput.Focus()
}

// closeCommand chiude la command line senza eseguire nulla (esc/invio vuoto).
func (m *Model) closeCommand() {
	m.cmdOpen = false
	m.cmdInput.Blur()
	m.resize()
}

// CommandOpen dice se la command line è aperta (tutti i tasti vanno
// all'input, il mouse è ignorato dalla radice).
func (m Model) CommandOpen() bool { return m.cmdOpen }

// PickerOpen dice se il filepicker di :save è aperto.
func (m Model) PickerOpen() bool { return m.pickerOpen }

// ModalOpen dice se un modale del thread è aperto (command line o filepicker):
// la radice lo usa per instradare TUTTI i tasti al thread e per ignorare il
// mouse (stessa gate di showHelp/labelOpen).
func (m Model) ModalOpen() bool { return m.cmdOpen || m.pickerOpen }

// executeCommand esegue la riga corrente e chiude la command line: il risultato
// è un messaggio di protocollo (feedback, SaveRequested, ThemeChanged) o
// l'apertura del filepicker. Invio su riga vuota chiude in silenzio.
func (m *Model) executeCommand() tea.Cmd {
	raw := m.cmdInput.Value()
	m.closeCommand()
	c := parseCommand(raw)
	if c.name == "" {
		return nil
	}
	switch c.name {
	case "save":
		return m.execSave(c.args)
	case "open":
		return m.execOpen(c.args)
	case "theme":
		return execTheme(c.args)
	default:
		return feedbackCmd(fmt.Sprintf("comando sconosciuto: %q — %s", c.name, commandList), true)
	}
}

// attachmentPart risolve n (1-based) sul PartIndex e il nome file dell'allegato
// n dell'ELENCO a fondo messaggio: il PartIndex è l'indice filtrato (coerenza
// garantita dal fix Piano 2), l'elenco è quello renderizzato dalle righe 📎.
func (m Model) attachmentPart(n int) (part int, filename string, err error) {
	sel := m.selMsg()
	if sel == nil {
		return 0, "", fmt.Errorf("messaggio non ancora caricato")
	}
	atts := sel.Attachments
	if n < 1 || n > len(atts) {
		if len(atts) == 0 {
			return 0, "", fmt.Errorf("allegato %d inesistente: il messaggio non ha allegati", n)
		}
		return 0, "", fmt.Errorf("allegato %d inesistente: il messaggio ne ha %d", n, len(atts))
	}
	return atts[n-1].PartIndex, atts[n-1].Filename, nil
}

// execSave esegue :save: percorso esplicito → SaveRequested subito; senza
// percorso → filepicker sulla directory corrente (la selezione produce il
// SaveRequested). Errori d'uso/range → feedback senza aprire nulla.
func (m *Model) execSave(args []string) tea.Cmd {
	if len(args) == 0 || len(args) > 2 {
		return feedbackCmd(saveUsage, true)
	}
	n, err := strconv.Atoi(args[0])
	if err != nil {
		return feedbackCmd(fmt.Sprintf("n non numerico: %q — %s", args[0], saveUsage), true)
	}
	part, _, err := m.attachmentPart(n)
	if err != nil {
		return feedbackCmd(err.Error(), true)
	}
	if len(args) == 2 {
		return m.saveRequested(SaveRequested{ID: m.selRef().ID, Part: part, Path: args[1]})
	}
	return m.openPicker(part)
}

// execOpen esegue :open: il destinatario è un tmpfile in os.TempDir col nome
// ORIGINALE dell'allegato (poi la radice apre con open/xdg-open come `o`).
func (m *Model) execOpen(args []string) tea.Cmd {
	if len(args) != 1 {
		return feedbackCmd(openUsage, true)
	}
	n, err := strconv.Atoi(args[0])
	if err != nil {
		return feedbackCmd(fmt.Sprintf("n non numerico: %q — %s", args[0], openUsage), true)
	}
	part, filename, err := m.attachmentPart(n)
	if err != nil {
		return feedbackCmd(err.Error(), true)
	}
	path := filepath.Join(os.TempDir(), filepath.Base(filename))
	return m.saveRequested(SaveRequested{ID: m.selRef().ID, Part: part, Path: path, Open: true})
}

// execTheme esegue :theme: senza argomenti elenca i temi (feedback
// informativo); con un nome carica il tema embedded e produce ThemeChanged
// con gli stili nuovi; tema inesistente → feedback di errore con l'elenco.
func execTheme(args []string) tea.Cmd {
	switch len(args) {
	case 0:
		return feedbackCmd("temi: "+strings.Join(theme.Names(), ", "), false)
	case 1:
		return func() tea.Msg {
			name := args[0]
			th, err := theme.Load(name)
			if err != nil {
				return CommandFeedback{
					Text:  fmt.Sprintf("tema %q non disponibile — temi: %s", name, strings.Join(theme.Names(), ", ")),
					IsErr: true,
				}
			}
			return ThemeChanged{Th: th, St: styles.New(th)}
		}
	default:
		return feedbackCmd(themeUsage, true)
	}
}

// openPicker configura e apre il filepicker bubbles sulla directory corrente:
// si selezionano file (salva su quel percorso) e directory (salva DENTRO con
// il nome originale); permessi e size sono nascosti per compattezza, gli
// stili arrivano dagli slot del tema. Restituisce la Init del picker (lettura
// della directory fuori dal loop UI).
func (m *Model) openPicker(part int) tea.Cmd {
	p := filepicker.New()
	p.ShowPermissions = false
	p.ShowSize = false
	p.FileAllowed = true
	p.DirAllowed = true
	p.Styles = themedPickerStyles(m.st)
	p = m.pickerHeight(p)
	m.picker = p
	m.pickerOpen = true
	m.pickPart = part
	return p.Init()
}

// pickerHeight dà al picker l'altezza che resta dopo header e hint (il picker
// sostituisce corpo e allegati nella vista).
func (m *Model) pickerHeight(p filepicker.Model) filepicker.Model {
	if m.height > headerLines+1 {
		p.SetHeight(m.height - headerLines - 1)
	}
	return p
}

// resolvePickPath converte la selezione del picker nel percorso di salvataggio:
// un file è il percorso stesso (assoluto), una directory riceve il nome
// originale dell'allegato.
func (m Model) resolvePickPath(sel string) (string, error) {
	abs, err := filepath.Abs(sel)
	if err != nil {
		return "", fmt.Errorf("percorso allegato: %w", err)
	}
	if info, err := os.Stat(abs); err == nil && info.IsDir() {
		return filepath.Join(abs, filepath.Base(m.selMsg().Attachments[m.pickPart].Filename)), nil
	}
	return abs, nil
}

// updatePicker instrada i messaggi al filepicker: esc annulla, invio conferma
// la selezione (SaveRequested), tutto il resto è navigazione.
func (m *Model) updatePicker(ev tea.Msg) tea.Cmd {
	if k, ok := ev.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "esc":
			m.pickerOpen = false
			return nil // annulla senza operare
		case "enter":
			var cmd tea.Cmd
			m.picker, cmd = m.picker.Update(ev) // il picker fissa il percorso selezionato
			if ok, sel := m.picker.DidSelectFile(ev); ok {
				m.pickerOpen = false
				path, err := m.resolvePickPath(sel)
				if err != nil {
					return feedbackCmd(err.Error(), true)
				}
				return m.saveRequested(SaveRequested{ID: m.selRef().ID, Part: m.selMsg().Attachments[m.pickPart].PartIndex, Path: path})
			}
			return cmd // navigazione (directory): la cmd ricarica le voci
		}
	}
	var cmd tea.Cmd
	m.picker, cmd = m.picker.Update(ev)
	return cmd
}

// updateCommand instrada i messaggi alla command line: enter esegue, esc
// annulla, il resto (testo, blink del cursore) va all'input.
func (m *Model) updateCommand(ev tea.Msg) tea.Cmd {
	if k, ok := ev.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "enter":
			return m.executeCommand()
		case "esc":
			m.closeCommand()
			return nil
		}
	}
	var cmd tea.Cmd
	m.cmdInput, cmd = m.cmdInput.Update(ev)
	return cmd
}

// saveRequested chiude i modali e produce il messaggio di richiesta.
func (m *Model) saveRequested(req SaveRequested) tea.Cmd {
	return func() tea.Msg { return req }
}

// feedbackCmd incapsula un CommandFeedback in una Cmd (il flusso dei messaggi
// resta uniforme: Update non produce mai messaggi direttamente).
func feedbackCmd(text string, isErr bool) tea.Cmd {
	return func() tea.Msg { return CommandFeedback{Text: text, IsErr: isErr} }
}

// SetStyles sostituisce gli stili e ri-renderizza ciò che è pre-stilizzato in
// New (header, corpo citato nel viewport, righe allegati) senza toccare lo
// stato (ref, scroll, modali aperti). Chiamata dalla radice al cambio tema.
func (m *Model) SetStyles(st styles.Styles) {
	m.st = st
	// Piano 15, Task 14: il tema cambia la chiave della cache (i corpi gia resi con
	// gli stili vecchi non si riusano) e la generazione (rendering in volo stantii).
	m.theme = themeKey(st)
	m.gen = nextGen()
	m.cache.resetPending()
	m.renderQ = nil
	m.refresh() // header, corpo (o titoli) e allegati con i nuovi stili
	m.cmdInput.SetStyles(commandInputStyles(st))
	if m.pickerOpen {
		m.picker.Styles = themedPickerStyles(st)
	}
}
