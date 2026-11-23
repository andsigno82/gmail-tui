// Package thread contiene la vista lettura messaggio della TUI: scheda
// d'intestazione con badge delle iniziali (Da/A/Oggetto/Data), corpo a blocchi
// (internal/msg/richtext + internal/ui/render) in bubbles/viewport v2, allegati
// come chip in coda (Piano 15, Task 14). L'app radice delega tasti e mouse a
// questo modello quando la vista attiva è il thread.
package thread

import (
	"fmt"
	"hash/fnv"
	"net/mail"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/filepicker"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/andsigno82/gmail-tui/internal/childenv"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/msg/richtext"
	"github.com/andsigno82/gmail-tui/internal/ui/render"
	"github.com/andsigno82/gmail-tui/internal/ui/styles"
)

// Layout (Piano 15, Task 14, D15-05): scheda d'intestazione di 6 righe (bordo
// superiore, Da/A/Oggetto/Data, bordo inferiore) + 1 riga vuota di separazione;
// dopo il corpo, 1 riga vuota + una riga per chip allegato (solo se ce ne sono).
// Prima del Task 14 erano 5 righe: LinkAt e ClickRow partono da questa costante.
const headerLines = 7

// mesiIT sono i nomi dei mesi abbreviati per la data assoluta dell'header.
var mesiIT = [...]string{"gen", "feb", "mar", "apr", "mag", "giu", "lug", "ago", "set", "ott", "nov", "dic"}

// BackMsg notifica all'app la richiesta di uscita dal thread (esc/q): la
// radice torna all'inbox PRESERVANDO lo stato della lista (nessun re-list).
type BackMsg struct{}

// OpenRequested è il risultato della preparazione di "apri nel browser": la
// Cmd di `o` ha scritto il tmpfile HTML (Path) o ha fallito (Err). La radice
// risponde con OpenCmd, che sospende la TUI per il browser di sistema.
type OpenRequested struct {
	Path string
	Err  error
}

// OpenedMsg arriva quando il browser è uscito (callback di tea.ExecProcess):
// l'esito finisce nella status bar.
type OpenedMsg struct {
	Err error
}

// Model è la vista thread: header pre-stilizzato + viewport del corpo +
// righe allegati. Il contenuto è pre-stilizzato (quote incluse) PRIMA di
// entrare nel viewport, che lo scorre senza toccare gli stili. Con la command
// line `:` aperta (cmdmode.go) una riga extra sotto il corpo ospita l'input;
// col filepicker di :save il picker sostituisce corpo e allegati.
type Model struct {
	id       msg.MessageID   // riga aperta: identità per anteprima e stantii (Piano 8, Task 10)
	conv     []entry         // messaggi in ordine cronologico; uno solo = vista singola
	sel      int             // messaggio selezionato: azioni, r/f, o, :save (default l'ultimo)
	starts   []int           // riga del titolo di ogni messaggio nel contenuto (solo conversazione)
	linkRows [][]render.Zone // zone cliccabili per riga del contenuto (indice = riga nel viewport, Piano 15, Task 14)
	vp       viewport.Model
	header   string          // scheda header pre-renderizzata (headerLines-1 righe) del messaggio selezionato
	attLines []string        // righe dei chip allegati pre-renderizzate del messaggio selezionato
	attZones [][]render.Zone // zone dei chip, una riga per chip
	st       styles.Styles
	width    int
	height   int

	// Rendering del corpo (Piano 15, Task 14, D15-T2/T5): cache in memoria
	// condivisa tra le copie del Model (puntatore), impronta del tema per la
	// chiave, generazione per scartare i risultati stantii e coda delle Cmd di
	// rendering in background (svuotata da RenderCmd).
	cache   *renderCache
	theme   string
	gen     int
	renderQ []tea.Cmd

	// Modalità comando `:` (Piano 4, Task 2): stato e logica in cmdmode.go.
	cmdOpen    bool            // command line aperta: i tasti vanno all'input
	cmdInput   textinput.Model // riga di comando inline sotto il corpo
	pickerOpen bool            // filepicker di :save aperto
	picker     filepicker.Model
	pickPart   int // PartIndex dell'allegato da salvare (n-1)
}

// entry è un messaggio della conversazione nella vista thread (Piano 8, Task 10).
type entry struct {
	ref      msg.MessageRef // copia locale (MarkRead e op la aggiornano)
	full     *msg.Message   // corpo completo (nil finché non caricato)
	expanded bool           // espanso: corpo visibile; compresso: una riga di titolo
}

// New costruisce la vista thread per un messaggio singolo (fallback senza
// X-GM-EXT-1: comportamento precedente al Piano 8, Task 10). full nil è
// difensivo: si rende solo l'header (usato anche prima del fetch).
func New(ref msg.MessageRef, full *msg.Message, st styles.Styles) Model {
	m := newBase(ref.ID, st)
	m.conv = []entry{{ref: ref, full: full, expanded: true}}
	m.refresh()
	return m
}

// NewConversation costruisce la vista della conversazione (Piano 8, Task 10):
// full in ordine cronologico, anchor è la riga aperta. Espansi: l'ultimo
// messaggio e i non letti; compressi: gli altri. Selezione di default: l'ultimo.
func NewConversation(anchor msg.MessageRef, full []*msg.Message, st styles.Styles) Model {
	refs := make([]msg.MessageRef, 0, len(full))
	bodies := make(map[msg.MessageID]*msg.Message, len(full))
	for _, f := range full {
		refs = append(refs, f.Ref)
		bodies[f.Ref.ID] = f
	}
	return NewLazy(anchor, refs, bodies, st)
}

// newBase prepara viewport (con il rebind dello scroll) e command line.
func newBase(id msg.MessageID, st styles.Styles) Model {
	m := Model{id: id, st: st, cache: newRenderCache(), theme: themeKey(st), gen: nextGen()}
	m.vp = viewport.New()
	m.vp.SoftWrap = false // il corpo va a capo in bodyRows, per tenere i link su piu righe (Richiesta di Andrea, 2026-11-23)
	// Rebind scroll (Piano 4, Task 3, decisione controller): il keymap default
	// di bubbles viewport lega f (page giù) e b (page su) — tasti che il
	// spec keymap riserva a inoltra (f) e libera (b). Restano pgup/pgdn/space
	// (pagina) e d/u (mezza pagina); f/b non significano più nulla qui.
	km := viewport.DefaultKeyMap()
	km.PageDown = key.NewBinding(key.WithKeys("pgdown", "pgdn", "space"), key.WithHelp("pgdn/space", "page giù"))
	km.PageUp = key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "page su"))
	m.vp.KeyMap = km
	m.cmdInput = newCommandInput(st)
	return m
}

// Update gestisce i modali del thread PRIMA della vista: con la command line
// o il filepicker aperti TUTTI i messaggi vanno a loro (tasti = testo/
// navigazione, come il prompt label della radice). Poi esc/q (BackMsg), `:`
// (apre la command line), `o` (open nel browser, solo con HTML) e delega al
// viewport, che gestisce nativamente j/k/frecce/paginazione e la rotella del
// mouse (MouseWheelEnabled di default, delta 3).
func (m Model) Update(ev tea.Msg) (Model, tea.Cmd) {
	mm := m
	// Risultato di un rendering in background (Piano 15, Task 14, D15-T5): vale
	// anche con la command line o il picker aperti, e se stantio e scartato.
	if r, ok := ev.(RenderedMsg); ok {
		mm.applyRendered(r)
		return mm, nil
	}
	if m.pickerOpen {
		cmd := mm.updatePicker(ev)
		return mm, cmd
	}
	if m.cmdOpen {
		cmd := mm.updateCommand(ev)
		return mm, cmd
	}
	// Link nel corpo (Richiesta di Andrea, 2026-11-23): il clic sinistro su un link
	// apre il browser senza toccare selezione o scorrimento; altrove il clic resta.
	if click, ok := ev.(tea.MouseClickMsg); ok && click.Button == tea.MouseLeft {
		if cmd, ok := mm.LinkClick(click.Mouse().X, click.Mouse().Y); ok {
			return mm, cmd
		}
	}
	// Conversazione (Piano 8, Task 10): j/k scelgono il messaggio, invio ne
	// alterna l'espansione, il clic sul titolo lo seleziona ed espande. Con un
	// solo messaggio restano i tasti di scorrimento di sempre.
	if m.multi() {
		switch ev := ev.(type) {
		case tea.KeyPressMsg:
			switch ev.String() {
			case "j", "down":
				return mm, mm.move(1)
			case "k", "up":
				return mm, mm.move(-1)
			case "enter":
				return mm, mm.toggle(mm.sel)
			}
		case tea.MouseClickMsg:
			if ev.Mouse().Button == tea.MouseLeft {
				if ok, cmd := mm.ClickRow(ev.Mouse().Y); ok {
					return mm, cmd
				}
			}
		}
	}
	switch ev := ev.(type) {
	case tea.KeyPressMsg:
		switch ev.String() {
		case "esc", "q":
			return m, func() tea.Msg { return BackMsg{} }
		case ":":
			return mm, mm.openCommand()
		case "o":
			return m, m.openInBrowser()
		}
	}
	var cmd tea.Cmd
	mm.vp, cmd = mm.vp.Update(ev) // la viewport v2 restituisce sempre cmd nil
	return mm, cmd
}

// View rende header + corpo scorrevole + allegati come stringa: le sub-viste
// v2 restano string, solo la radice le incapsula in tea.NewView. Col picker
// aperto il picker sostituisce corpo e allegati (con hint di conferma); con
// la command line aperta l'input sta su una riga in fondo.
func (m Model) View() string {
	var b strings.Builder
	b.WriteString(m.header)
	if m.pickerOpen {
		b.WriteString("\n\n")
		b.WriteString(m.picker.View())
		b.WriteString("\n")
		b.WriteString(m.st.HelpView.Render(pickerHint))
		return b.String()
	}
	b.WriteString("\n\n")
	b.WriteString(m.vp.View())
	if len(m.attLines) > 0 {
		b.WriteString("\n\n")
		b.WriteString(strings.Join(m.attLines, "\n"))
	}
	if m.cmdOpen {
		b.WriteString("\n")
		b.WriteString(m.cmdInput.View())
	}
	return b.String()
}

// SetSize propaga le dimensioni del corpo (la radice riserva l'ultima riga
// alla status bar): la viewport ottiene l'altezza che resta dopo header,
// allegati e — se aperta — command line. Ricevitore puntatore: i modelli
// bubbles (viewport/filepicker) mutano lo stato con i loro Set*.
func (m *Model) SetSize(width, height int) {
	if width != m.width {
		// Nuova larghezza = nuova generazione: i rendering in volo per la larghezza
		// precedente saranno scartati e quelli ancora necessari si rilanciano.
		m.gen = nextGen()
		m.cache.resetPending()
		m.renderQ = nil // le Cmd accodate per la larghezza precedente sono gia stantie
	}
	m.width, m.height = width, height
	m.refresh() // i titoli e il corpo a capo dipendono dalla larghezza
	m.resize()
	if m.pickerOpen {
		m.picker = m.pickerHeight(m.picker)
	}
}

// resize ricalcola l'altezza della viewport dai vincoli correnti (chiamata da
// SetSize e all'apertura/chiusura della command line, che occupa/ffre una riga).
func (m *Model) resize() {
	used := headerLines
	if len(m.attLines) > 0 {
		used += 1 + len(m.attLines)
	}
	if m.cmdOpen {
		used++
	}
	vh := max(0, m.height-used)
	m.vp.SetWidth(max(0, m.width))
	m.vp.SetHeight(vh)
}

// MarkRead segna letti tutti i messaggi della conversazione (la radice lo fa
// all'apertura e nell'anteprima, Piano 8, Task 10): l'header resta coerente
// senza re-list (il refresh lista avviene via Watch/nuova ricerca; il back
// preserva la lista). Nella vista singola è l'unico messaggio, come prima.
func (m *Model) MarkRead() {
	for i := range m.conv {
		m.conv[i].ref.Unread = false
	}
	if m.multi() {
		m.refresh()
	}
}

// SetRef sostituisce la copia locale del ref del messaggio selezionato: sync
// dopo le mutazioni da thread (Task 7) — l'header non rende i flag, ma i toggle
// consecutivi dell'utente leggono questo stato. Ricevitore puntatore.
func (m *Model) SetRef(ref msg.MessageRef) {
	if len(m.conv) == 0 {
		return
	}
	m.conv[m.sel].ref = ref
	if m.multi() {
		m.refresh()
	}
}

// Ref restituisce la copia locale del ref del messaggio selezionato.
func (m Model) Ref() msg.MessageRef { return m.selRef() }

// Loaded dice se la vista ha almeno un corpo caricato (false durante il fetch).
// Con la conversazione lazy basta un corpo: il messaggio selezionato può ancora
// essere in caricamento senza nascondere i messaggi già visibili (Piano 8, Task 13).
func (m Model) Loaded() bool {
	for _, e := range m.conv {
		if e.full != nil {
			return true
		}
	}
	return false
}

// Msg restituisce il messaggio selezionato completo (nil finché non caricato):
// serve alla radice per i composer precompilati r/f (Piano 4, Task 3) — il
// reply/forward usa il msg già caricato senza rifetch. Con una conversazione è il
// messaggio selezionato, di default l'ultimo (Piano 8, Task 10).
func (m Model) Msg() *msg.Message { return m.selMsg() }

// AnchorID è l'ID della riga aperta: identifica la conversazione per anteprima
// e scarto degli stantii, anche quando la selezione cambia.
func (m Model) AnchorID() msg.MessageID { return m.id }

// UnreadIDs elenca gli ID dei messaggi non letti della conversazione: all'apertura
// e nell'anteprima si segnano letti tutti insieme (Piano 8, Task 10).
func (m Model) UnreadIDs() []msg.MessageID {
	var out []msg.MessageID
	for _, e := range m.conv {
		if e.ref.Unread {
			out = append(out, e.ref.ID)
		}
	}
	return out
}

// YOffset è la posizione di scorrimento del corpo (seam per i test).
func (m Model) YOffset() int { return m.vp.YOffset() }

// badgeW e la larghezza in colonne del badge delle iniziali; headerOverhead le
// colonne della scheda fuori dal testo: due bordi, due spazi interni, il badge e
// uno spazio tra badge e testo.
const (
	badgeW         = 4
	headerOverhead = 2 + 2 + badgeW + 1
)

// initials ricava le iniziali del mittente (D15-05): prima e ultima parola del
// nome (o della parte locale dell'indirizzo, spezzata su . _ -); una sola parola
// = una sola lettera. Sempre maiuscole; "?" se non c'e nulla di leggibile.
func initials(from string) string {
	name := from
	if a, err := mail.ParseAddress(from); err == nil {
		name = a.Name
		if name == "" {
			name, _, _ = strings.Cut(a.Address, "@")
		}
	}
	words := strings.FieldsFunc(name, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	first := func(w string) string {
		for _, r := range w {
			return string(unicode.ToUpper(r))
		}
		return ""
	}
	switch len(words) {
	case 0:
		return "?"
	case 1:
		return first(words[0])
	}
	return first(words[0]) + first(words[len(words)-1])
}

// badge rende il badge delle iniziali (badgeW colonne) con un colore del tema
// scelto in modo stabile dall'indirizzo, come le label (D15-05).
func (m Model) badge(from string) string {
	text := " " + initials(from)
	text += strings.Repeat(" ", max(0, badgeW-lipgloss.Width(text)))
	if len(m.st.BadgeAvatar) == 0 {
		return text
	}
	key := strings.ToLower(from)
	if a, err := mail.ParseAddress(from); err == nil {
		key = strings.ToLower(a.Address)
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return m.st.BadgeAvatar[int(h.Sum32()%uint32(len(m.st.BadgeAvatar)))].Render(text)
}

// renderHeader compone la scheda d'intestazione (Piano 15, Task 14, D15-05):
// bordo arrotondato, badge delle iniziali a sinistra e le coppie chiave/valore
// (etichette in ThreadHeaderKey, valori in ThreadHeader: il padding dello stile fa
// da spazio). Larga come la vista; prima del primo SetSize larga quanto il testo.
// I valori troppo lunghi si troncano con "…". Il bordo e in ThreadHeaderKey (slot
// muted): nessun colore fisso.
func (m Model) renderHeader() string {
	to := ""
	if sel := m.selMsg(); sel != nil {
		to = strings.Join(sel.To, ", ")
	}
	ref := m.selRef()
	pairs := [][2]string{{"Da", ref.From}, {"A", to}, {"Oggetto", ref.Subject}, {"Data", FormatDate(ref.Date)}}

	textW := 0 // larghezza del testo disponibile; 0 = quella naturale
	if m.width > 0 {
		textW = max(1, m.width-headerOverhead)
	}
	kvs := make([]string, len(pairs))
	natural := 0
	for i, p := range pairs {
		v := cleanHeader(p[1])
		if textW > 0 {
			// chiave + ":" a sinistra, due colonne di padding del valore.
			v = render.TruncCols(v, max(1, textW-lipgloss.Width(p[0]+":")-2))
		}
		kvs[i] = m.st.ThreadHeaderKey.Render(p[0]+":") + m.st.ThreadHeader.Render(v)
		natural = max(natural, lipgloss.Width(kvs[i]))
	}
	if textW == 0 {
		textW = natural
	}

	border := func(s string) string { return m.st.ThreadHeaderKey.Render(s) }
	bar := strings.Repeat("─", textW+headerOverhead-2)
	lines := []string{border("╭" + bar + "╮")}
	for i, kv := range kvs {
		cell := strings.Repeat(" ", badgeW)
		if i == 0 {
			cell = m.badge(ref.From)
		}
		pad := strings.Repeat(" ", max(0, textW-lipgloss.Width(kv)))
		lines = append(lines, border("│")+" "+cell+" "+kv+pad+" "+border("│"))
	}
	lines = append(lines, border("╰"+bar+"╯"))
	return strings.Join(lines, "\n")
}

// cleanHeader toglie dai valori della scheda controlli e caratteri di formato (a
// capo compresi): un Subject o un From non fidato non deve rompere le righe della
// scheda, iniettare sequenze nel terminale ne mascherarsi con bidi o zero-width.
// Filtro unico (richtext.Unsafe, revisione Piano 15, punto 4).
func cleanHeader(s string) string { return richtext.CleanLine(s) }

// renderAttachments prepara i chip degli allegati (Piano 15, Task 14, D15-06):
// un render.Attachment per allegato, con Index = posizione in full.Attachments.
// Le zone alimentano ZoneAt/LinkClick (AttachmentClickMsg).
func (m Model) renderAttachments(full *msg.Message) render.Out {
	c := render.Ctx{Width: m.width, St: m.st}
	outs := make([]render.Out, 0, len(full.Attachments))
	for i, a := range full.Attachments {
		outs = append(outs, render.Attachment(a.Filename, a.ContentType, a.Size, i, c))
	}
	return render.Stack(outs...)
}

// openInBrowser produce la Cmd per `o`: solo se il messaggio ha un corpo HTML
// (BodyHTML, popolato anche per i multipart/alternative — non solo per i
// messaggi solo-HTML). La Cmd scrive il tmpfile .html (I/O locale istantaneo,
// fuori dal loop Update) e restituisce OpenRequested; la radice chiama
// OpenCmd per l'exec vero.
func (m Model) openInBrowser() tea.Cmd {
	sel := m.selMsg()
	if sel == nil || sel.BodyHTML == "" {
		return nil
	}
	html := sel.BodyHTML
	return func() tea.Msg {
		path, err := writeHTMLTemp(html)
		return OpenRequested{Path: path, Err: err}
	}
}

// OpenCmd è la Cmd di exec: sospende la TUI (tea.ExecProcess), apre path con
// il browser di sistema e riporta l'esito in OpenedMsg per la status bar.
func OpenCmd(path string) tea.Cmd {
	return tea.ExecProcess(browserCmd(path), func(err error) tea.Msg {
		return OpenedMsg{Err: err}
	})
}

// browserCmd sceglie il comando di sistema: open su darwin, xdg-open altrove.
func browserCmd(path string) *exec.Cmd {
	var c *exec.Cmd
	if runtime.GOOS == "darwin" {
		c = exec.Command("open", path)
	} else {
		c = exec.Command("xdg-open", path)
	}
	// Ambiente filtrato: la password App e le GMAIL_TUI_* non escono verso il browser (childenv).
	c.Env = childenv.Filtered()
	return c
}

// writeHTMLTemp scrive l'HTML del messaggio in un tmpfile .html e restituisce
// il percorso. In caso di errore di scrittura il file parziale è rimosso.
func writeHTMLTemp(html string) (string, error) {
	f, err := os.CreateTemp("", "gmail-tui-*.html")
	if err != nil {
		return "", fmt.Errorf("tmpfile html: %w", err)
	}
	if _, err := f.WriteString(html); err != nil {
		name := f.Name()
		_ = f.Close()
		_ = os.Remove(name)
		return "", fmt.Errorf("scrivi tmpfile html: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("chiudi tmpfile html: %w", err)
	}
	return f.Name(), nil
}

// FormatDate rende la data assoluta in italiano, deterministica:
// "20 nov 2026 10:00" (nel fuso del messaggio). Esportata per riuso:
// l'header citazione del composer (Piano 4, Task 3) usa lo stesso formato
// dell'header del thread.
func FormatDate(d time.Time) string {
	if d.IsZero() {
		return "—"
	}
	return fmt.Sprintf("%d %s %d %02d:%02d",
		d.Day(), mesiIT[d.Month()-1], d.Year(), d.Hour(), d.Minute())
}
