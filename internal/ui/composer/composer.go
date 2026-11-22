// Package composer contiene il composer della TUI (Piano 4, Task 3): campi
// stile pop (Da/A/Cc/Oggetto/Corpo/Allegati), navigazione tab/frecce,
// filepicker multi-allegato e bozza msg.Outgoing. Il modello NON tocca il
// MailStore né il sender: produce il Draft e il messaggio di protocollo
// SendRequestedMsg — conferma e invio sono della radice (Task 4).
//
// Adattato da charmbracelet/pop (MIT).
package composer

import (
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/filepicker"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/styles"
)

// Field identifica il campo attivo del composer (ordine di navigazione).
// FieldFrom è il mittente precompilato dalla config: display readonly, mai
// focalizzato (il campo esiste nell'enum per completezza del routing).
type Field int

const (
	FieldFrom Field = iota
	FieldTo
	FieldCc
	FieldBcc // Ccn (Piano 9, decisione del coordinatore): con Cc, toggle ctrl+e
	FieldSubject
	FieldBody
	FieldAttachments
)

// ComposerConfig raccoglie i dati precompilati del composer: From/Signature
// dall'identità dell'utente (config), To/Cc/Subject/Body per reply/forward,
// QuotedBody è la citazione iniziale nel textarea (appesa sotto Body se
// entrambe presenti), Attachments i percorsi pre-allegati.
type ComposerConfig struct {
	From, Signature            string
	To, Cc, Bcc, Subject, Body string
	QuotedBody                 string
	Attachments                []string
}

// SendRequestedMsg è prodotto da ctrl+s: contiene la bozza completa (firma
// inclusa). La radice decide conferma e invio (Task 4).
type SendRequestedMsg struct{ Out msg.Outgoing }

// hint del composer (i tasti operazione dell'app non valgono qui: dentro il
// composer sono testo) e del filepicker allegati.
const (
	composerHint = "tab campi · ctrl+e cc · ctrl+s invia · esc annulla"
	pickerHint   = "invio aggiungi (resta aperto per più file) · esc annulla"
)

// Model è il composer: campi mono-riga (textinput), corpo markdown
// (textarea) e allegati con filepicker multi-add. Lo stato cancel è la
// richiesta di annullamento (esc): la radice la traduce in back diretto o
// conferma di discard a seconda che il corpo sia vuoto o no.
type Model struct {
	st            styles.Styles
	width, height int

	from      string // display readonly (mai focalizzato)
	signature string // appended nel Draft stile pop

	to, cc, bcc, subject textinput.Model
	body                 textarea.Model

	ccVisible bool // toggle ctrl+e (da config se Cc precompilato)
	focus     Field
	cancel    bool // richiesta di annullamento (esc), consumata da un tasto vero

	attachments []string
	attSel      int // allegato selezionato per `d` (rimozione)

	picker     filepicker.Model
	pickerOpen bool
}

// New costruisce il composer per la config data. Il focus iniziale va al
// primo campo vuoto (stile pop): nuovo messaggio → A, reply/forward
// precompilati → Corpo. La cmd di blink del focus iniziale è scartata (New
// non restituisce cmd: la radice apre il composer senza cmd aggiuntive).
func New(cfg ComposerConfig, st styles.Styles) Model {
	m := Model{
		st:          st,
		from:        cfg.From,
		signature:   cfg.Signature,
		ccVisible:   cfg.Cc != "" || cfg.Bcc != "",
		attachments: append([]string(nil), cfg.Attachments...),
	}
	m.to = newInput(st, "destinatario, separati da virgole")
	m.to.SetValue(cfg.To)
	m.cc = newInput(st, "cc, separati da virgole")
	m.cc.SetValue(cfg.Cc)
	m.bcc = newInput(st, "ccn, separati da virgole")
	m.bcc.SetValue(cfg.Bcc)
	m.subject = newInput(st, "oggetto")
	m.subject.SetValue(cfg.Subject)
	m.body = textarea.New()
	m.body.SetValue(initialBody(cfg))
	m.body.Placeholder = "messaggio (markdown)"
	m.body.SetStyles(themedTextareaStyles(st))
	m.focus = firstEmpty(m)
	_ = m.focusField(m.focus)
	return m
}

// initialBody unisce Body e QuotedBody (la citazione sotto il testo precompilato).
func initialBody(cfg ComposerConfig) string {
	switch {
	case cfg.Body == "":
		return cfg.QuotedBody
	case cfg.QuotedBody == "":
		return cfg.Body
	default:
		return cfg.Body + "\n" + cfg.QuotedBody
	}
}

// newInput costruisce un textinput del composer: prompt assente (l'etichetta
// è renderizzata dalla View col tema), placeholder attenuato, cursore blocco.
func newInput(st styles.Styles, placeholder string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 512
	var s textinput.Styles
	s.Focused.Placeholder = st.ListDate
	s.Blurred.Placeholder = st.ListDate
	s.Cursor = textinput.CursorStyle{Shape: tea.CursorBlock, Blink: true}
	ti.SetStyles(s)
	return ti
}

// themedTextareaStyles mappa gli slot del tema sugli stili del textarea
// (i default di bubbles hardcodano hex: vale il vincolo "solo slot del tema").
func themedTextareaStyles(st styles.Styles) textarea.Styles {
	var s textarea.Styles
	s.Focused.Text = st.ListNormal
	s.Focused.Placeholder = st.ListDate
	s.Focused.EndOfBuffer = st.ListDate
	s.Blurred.Text = st.ListNormal
	s.Blurred.Placeholder = st.ListDate
	s.Blurred.EndOfBuffer = st.ListDate
	s.Cursor.Shape = tea.CursorBlock
	s.Cursor.Blink = true
	return s
}

// firstEmpty restituisce il primo campo vuoto nell'ordine di navigazione
// (To, Cc se visibile, Subject, Body); tutti precompilati → Body (lo stile
// pop: nel reply si scrive sopra la citazione).
func firstEmpty(m Model) Field {
	if m.to.Value() == "" {
		return FieldTo
	}
	if m.ccVisible && m.cc.Value() == "" {
		return FieldCc
	}
	if m.ccVisible && m.bcc.Value() == "" {
		return FieldBcc
	}
	if m.subject.Value() == "" {
		return FieldSubject
	}
	if m.body.Value() == "" {
		return FieldBody
	}
	return FieldBody
}

// visibleFields è l'ordine di navigazione (From readonly escluso).
func (m Model) visibleFields() []Field {
	f := []Field{FieldTo}
	if m.ccVisible {
		f = append(f, FieldCc, FieldBcc)
	}
	return append(f, FieldSubject, FieldBody, FieldAttachments)
}

// nextField / prevField ciclano i campi visibili (wrap ai capi).
func (m Model) nextField() Field {
	fs := m.visibleFields()
	for i, f := range fs {
		if f == m.focus {
			return fs[(i+1)%len(fs)]
		}
	}
	return fs[0]
}

func (m Model) prevField() Field {
	fs := m.visibleFields()
	for i, f := range fs {
		if f == m.focus {
			return fs[(i+len(fs)-1)%len(fs)]
		}
	}
	return fs[0]
}

// focusField sposta il focus sul campo dato (blur su tutti gli altri).
func (m *Model) focusField(f Field) tea.Cmd {
	m.focus = f
	m.to.Blur()
	m.cc.Blur()
	m.bcc.Blur()
	m.subject.Blur()
	m.body.Blur()
	switch f {
	case FieldTo:
		return m.to.Focus()
	case FieldCc:
		return m.cc.Focus()
	case FieldBcc:
		return m.bcc.Focus()
	case FieldSubject:
		return m.subject.Focus()
	case FieldBody:
		return m.body.Focus()
	}
	return nil
}

// Update instrada i messaggi al composer. Con il filepicker aperto TUTTI i
// messaggi vanno al picker (esc chiude, invio aggiunge — multi-add). Nei
// campi: esc chiede il cancel, tab/shift+tab e frecce navigano (nel corpo
// le frecce spettano al textarea), ctrl+e toggle Cc, ctrl+s produce la
// bozza; QUALSIASI altro tasto è testo del campo focalizzato e consuma
// l'eventuale richiesta di cancel.
func (m Model) Update(ev tea.Msg) (Model, tea.Cmd) {
	mm := m
	if m.pickerOpen {
		return mm.updatePicker(ev)
	}
	if k, ok := ev.(tea.KeyPressMsg); ok {
		// QUALSIASI tasto vero consuma l'eventuale richiesta di cancel (dopo
		// la ripresa dal modale discard della radice, nessun tasto — anche
		// tab o le frecce, che escono in anticipo qui sotto — deve ri-aprire
		// la conferma all'insaputa dell'utente); esc la ri-arma sotto.
		mm.cancel = false
		switch k.String() {
		case "esc":
			mm.cancel = true
			return mm, nil
		case "tab":
			return mm, mm.focusField(mm.nextField())
		case "shift+tab":
			return mm, mm.focusField(mm.prevField())
		case "ctrl+e":
			return mm, mm.toggleCc()
		case "ctrl+s":
			return mm, sendCmd(mm.Draft())
		}
		if mm.focus == FieldAttachments {
			switch k.String() {
			case "enter":
				return mm, mm.openPicker()
			case "d":
				return mm.removeSelected()
			case "up", "k":
				mm.attSel = max(0, mm.attSel-1)
				return mm, nil
			case "down", "j":
				mm.attSel = min(len(mm.attachments)-1, mm.attSel+1)
				return mm, nil
			}
			return mm, nil // gli altri tasti non significano nulla sulla lista
		}
		// Nei campi mono-riga su/giù navigano i campi (nel corpo spettano al
		// textarea, che riceve tutto il resto sotto).
		if mm.focus != FieldBody {
			switch k.String() {
			case "up":
				return mm, mm.focusField(mm.prevField())
			case "down":
				return mm, mm.focusField(mm.nextField())
			}
		}
	}
	var cmd tea.Cmd
	switch mm.focus {
	case FieldTo:
		mm.to, cmd = mm.to.Update(ev)
	case FieldCc:
		mm.cc, cmd = mm.cc.Update(ev)
	case FieldBcc:
		mm.bcc, cmd = mm.bcc.Update(ev)
	case FieldSubject:
		mm.subject, cmd = mm.subject.Update(ev)
	case FieldBody:
		mm.body, cmd = mm.body.Update(ev)
	}
	return mm, cmd
}

// toggleCc mostra/nasconde il campo Cc (ctrl+e): alla comparsa il focus va
// su Cc, alla scomparsa torna su A se stava lì (il campo non esiste più).
func (m *Model) toggleCc() tea.Cmd {
	m.cancel = false
	m.ccVisible = !m.ccVisible
	if m.ccVisible {
		return m.focusField(FieldCc)
	}
	if m.focus == FieldCc || m.focus == FieldBcc {
		return m.focusField(FieldTo)
	}
	return nil
}

// Draft raccoglie i campi correnti in msg.Outgoing: To/Cc letti come lista RFC
// 5322 (msg.ParseRecipientField, Piano 7.1 fix code-review: la virgola nel nome
// quotato non spezza; un campo non parsabile resta intero e la conferma lo
// rifiuta citando "A"/"Cc"), oggetto trimmato, corpo tal
// quale e firma appended stile pop corpo + "\n\n--\n" + firma (solo se la
// firma non è vuota). Il Cc nascosto NON finisce nella bozza (l'utente non
// lo vede → non va in invio silenziosamente).
func (m Model) Draft() msg.Outgoing {
	out := msg.Outgoing{
		From:        m.from,
		To:          msg.ParseRecipientField(m.to.Value()),
		Subject:     strings.TrimSpace(m.subject.Value()),
		BodyText:    m.body.Value(),
		Attachments: append([]string(nil), m.attachments...),
		Markdown:    true,
	}
	if m.ccVisible {
		out.Cc = msg.ParseRecipientField(m.cc.Value())
		out.Bcc = msg.ParseRecipientField(m.bcc.Value())
	}
	if m.signature != "" {
		out.BodyText += "\n\n--\n" + m.signature
	}
	return out
}

// Empty dice se la bozza non ha nulla da salvare (Piano 9, D9-3): nessun
// destinatario (A, Cc, Ccn), nessun oggetto, nessun allegato, corpo vuoto.
// Il corpo è quello grezzo: la firma non conta.
func (m Model) Empty() bool {
	return m.to.Value() == "" && m.cc.Value() == "" && m.bcc.Value() == "" &&
		m.subject.Value() == "" && len(m.attachments) == 0 && strings.TrimSpace(m.body.Value()) == ""
}

// BodyText è il corpo GREZZO del textarea, senza la firma appended (seam per
// la radice: la conferma discard chiede conferma solo se l'utente ha scritto
// qualcosa — un corpo vuoto con firma sarebbe sempre "non vuoto" via Draft).
func (m Model) BodyText() string { return m.body.Value() }

// CancelRequested dice se esc ha chiesto l'annullamento: la radice lo legge
// subito dopo aver delegato un tasto al composer (back diretto se il corpo è
// vuoto, conferma discard altrimenti). QUALSIASI altro tasto la consuma.
func (m Model) CancelRequested() bool { return m.cancel }

// sendCmd incapsula la bozza in una Cmd (il flusso dei messaggi resta
// uniforme: Update non produce mai messaggi direttamente).
func sendCmd(out msg.Outgoing) tea.Cmd {
	return func() tea.Msg { return SendRequestedMsg{Out: out} }
}

// --- allegati -----------------------------------------------------------------

// openPicker apre il filepicker sulla directory corrente: solo file (una
// directory non si allega), stili dagli slot del tema. Restituisce la Init
// (lettura della directory fuori dal loop UI).
func (m *Model) openPicker() tea.Cmd {
	m.cancel = false
	p := filepicker.New()
	p.ShowPermissions = false
	p.ShowSize = false
	p.FileAllowed = true
	p.DirAllowed = false
	p.Styles = themedPickerStyles(m.st)
	m.picker = p
	m.pickerOpen = true
	m.resizePicker()
	return p.Init()
}

// themedPickerStyles mappa gli slot del tema sugli stili del filepicker.
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

// resizePicker dà al picker l'altezza che resta dopo i campi e l'hint.
func (m *Model) resizePicker() {
	if m.height > reservedLines(*m)+1 {
		m.picker.SetHeight(m.height - reservedLines(*m) - 1)
	}
}

// updatePicker instrada i messaggi al filepicker: esc chiude senza aggiungere,
// invio su un file lo aggiunge alla lista (percorso assoluto) e RESTA aperto
// per il multi-add (esc per finire); invio su directory naviga.
func (m Model) updatePicker(ev tea.Msg) (Model, tea.Cmd) {
	if k, ok := ev.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "esc":
			m.pickerOpen = false
			return m, nil
		case "enter":
			var cmd tea.Cmd
			m.picker, cmd = m.picker.Update(ev) // il picker fissa il percorso selezionato
			if ok, sel := m.picker.DidSelectFile(ev); ok {
				abs, err := filepath.Abs(sel)
				if err != nil {
					return m, cmd
				}
				m.attachments = append(m.attachments, abs)
				m.attSel = len(m.attachments) - 1
				return m, cmd // multi-add: il picker resta aperto
			}
			return m, cmd
		}
	}
	var cmd tea.Cmd
	m.picker, cmd = m.picker.Update(ev)
	return m, cmd
}

// removeSelected rimuove l'allegato sotto la selezione (tasto `d` sul campo
// Allegati); lista vuota → no-op.
func (m Model) removeSelected() (Model, tea.Cmd) {
	m.cancel = false
	if len(m.attachments) == 0 {
		return m, nil
	}
	m.attachments = append(m.attachments[:m.attSel], m.attachments[m.attSel+1:]...)
	if m.attSel >= len(m.attachments) {
		m.attSel = max(0, len(m.attachments)-1)
	}
	return m, nil
}

// --- layout ---------------------------------------------------------------------

// reservedLines è il numero di righe di view che circondano il corpo (Da, A,
// [Cc], Oggetto, riga vuota, riga vuota dopo il corpo, Allegati, [lista],
// riga vuota, hint).
func reservedLines(m Model) int {
	n := 4 // Da, A, Oggetto, riga vuota
	if m.ccVisible {
		n += 2 // Cc e Ccn
	}
	n += 2 // riga vuota dopo il corpo + riga Allegati
	if len(m.attachments) > 0 {
		n += len(m.attachments)
	}
	n += 2 // riga vuota prima dell'hint + hint
	return n
}

// SetSize propaga le dimensioni del corpo (la radice riserva l'ultima riga
// alla status bar): larghezza agli input mono-riga e al corpo, altezza al
// corpo (minimo 3 righe per restare usabile). Ricevitore puntatore: i modelli
// bubbles mutano lo stato interno con i loro Set*.
func (m *Model) SetSize(width, height int) {
	m.width, m.height = width, height
	w := max(0, width-12) // l'etichetta più lunga ("Oggetto: ") + spazio
	m.to.SetWidth(w)
	m.cc.SetWidth(w)
	m.bcc.SetWidth(w)
	m.subject.SetWidth(w)
	m.body.SetWidth(max(0, width))
	m.body.SetHeight(max(3, height-reservedLines(*m)))
	if m.pickerOpen {
		m.resizePicker()
	}
}

// View rende i campi con etichette italiane (il campo focalizzato ha
// l'etichetta in Accent), il corpo, la lista allegati con la selezione e la
// riga d'uso. Col filepicker aperto il picker sostituisce i campi (con hint
// di conferma), come nel thread.
func (m Model) View() string {
	if m.pickerOpen {
		return m.picker.View() + "\n" + m.st.HelpView.Render(pickerHint)
	}
	var b strings.Builder
	label := func(k string, f Field) string {
		st := m.st.ThreadHeaderKey
		if m.focus == f {
			st = m.st.SearchInput
		}
		return st.Render(k + ":")
	}
	b.WriteString(label("Da", FieldFrom) + " " + m.st.ListNormal.Render(m.from) + "\n")
	b.WriteString(label("A", FieldTo) + " " + m.to.View() + "\n")
	if m.ccVisible {
		b.WriteString(label("Cc", FieldCc) + " " + m.cc.View() + "\n")
		b.WriteString(label("Ccn", FieldBcc) + " " + m.bcc.View() + "\n")
	}
	b.WriteString(label("Oggetto", FieldSubject) + " " + m.subject.View() + "\n")
	b.WriteString("\n")
	b.WriteString(m.body.View())
	b.WriteString("\n\n")
	b.WriteString(m.st.SearchInput.Render("Allegati:"))
	if len(m.attachments) == 0 {
		b.WriteString(" " + m.st.ListDate.Render("(nessuno — invio per aggiungere)"))
	} else {
		b.WriteString("\n")
		for i, a := range m.attachments {
			marker, st := "  ", m.st.ListNormal
			if i == m.attSel && m.focus == FieldAttachments {
				marker, st = "> ", m.st.ListSelected
			}
			b.WriteString(m.st.ListDate.Render(marker) + st.Render("📎 "+a) + "\n")
		}
	}
	b.WriteString("\n" + m.st.HelpView.Render(composerHint))
	return b.String()
}
