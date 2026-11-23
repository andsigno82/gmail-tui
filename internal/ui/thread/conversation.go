package thread

// Vista conversazione stile Gmail (Piano 8, Task 10): più messaggi nello stesso
// viewport, i letti compressi su una riga di titolo (mittente, data), l'ultimo e
// i non letti espansi. Caricamento lazy (Piano 8, Task 13): il corpo dei messaggi
// compressi non è scaricato all'apertura; si chiede (BodyRequested) quando un
// messaggio viene selezionato o espanso. Un solo messaggio resta la vista di sempre.

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/render"
	"github.com/andsigno82/gmail-tui/internal/ui/styles"
)

// BodyRequested chiede alla radice il corpo di un messaggio della conversazione
// (Piano 8, Task 13): emesso quando il messaggio viene selezionato o espanso senza
// corpo. Anchor è la riga aperta, così la radice scarta le risposte stantie.
type BodyRequested struct {
	ID     msg.MessageID
	Anchor msg.MessageID
}

// isDefaultExpanded dice se il messaggio i di refs è espanso all'apertura: l'ultimo
// e i non letti (Piano 8, Task 10).
func isDefaultExpanded(i int, refs []msg.MessageRef) bool {
	return i == len(refs)-1 || refs[i].Unread
}

// DefaultExpanded elenca gli ID dei messaggi espansi all'apertura: la radice
// scarica i loro corpi, gli altri restano senza corpo (Piano 8, Task 13).
func DefaultExpanded(refs []msg.MessageRef) []msg.MessageID {
	var out []msg.MessageID
	for i, r := range refs {
		if isDefaultExpanded(i, refs) {
			out = append(out, r.ID)
		}
	}
	return out
}

// NewLazy costruisce la vista della conversazione dalle righe refs (in ordine
// cronologico) e dai soli corpi già caricati in bodies (Piano 8, Task 13). anchor
// è la riga aperta. Selezione di default: l'ultimo messaggio.
func NewLazy(anchor msg.MessageRef, refs []msg.MessageRef, bodies map[msg.MessageID]*msg.Message, st styles.Styles) Model {
	return NewLazyCached(anchor, refs, bodies, st, nil)
}

// NewLazyCached e NewLazy con la cache di rendering condivisa rc: l'app ne crea una
// sola (NewRenderCache) e la passa a ogni thread, cosi i corpi gia resi valgono
// anche tra messaggi diversi (D15-T2, revisione 2026-11-23). nil = cache privata.
func NewLazyCached(anchor msg.MessageRef, refs []msg.MessageRef, bodies map[msg.MessageID]*msg.Message, st styles.Styles, rc *RenderCache) Model {
	if len(refs) == 0 {
		m := newBaseOn(anchor.ID, st, rc) // come New
		m.conv = []entry{{ref: anchor, full: bodies[anchor.ID], expanded: true}}
		m.refresh()
		return m
	}
	m := newBaseOn(anchor.ID, st, rc)
	for i, r := range refs {
		m.conv = append(m.conv, entry{ref: r, full: bodies[r.ID], expanded: isDefaultExpanded(i, refs)})
	}
	m.sel = len(refs) - 1
	m.refresh()
	return m
}

// SetBody installa il corpo arrivato per il messaggio id (risposta a BodyRequested).
// Un corpo per un messaggio assente o nil non cambia nulla.
func (m *Model) SetBody(id msg.MessageID, full *msg.Message) {
	if full == nil {
		return
	}
	for i := range m.conv {
		if m.conv[i].ref.ID == id {
			m.conv[i].full = full
		}
	}
	m.refresh() // il corpo a capo dipende dalla larghezza anche nella vista singola
}

// multi dice se la vista è una conversazione (più di un messaggio): il ramo
// nuovo di j/k/invio/clic e il contenuto a titoli si attivano solo allora.
func (m Model) multi() bool { return len(m.conv) > 1 }

// selRef è la copia locale del ref del messaggio selezionato (zero senza messaggi).
func (m Model) selRef() msg.MessageRef {
	if len(m.conv) == 0 {
		return msg.MessageRef{}
	}
	return m.conv[m.sel].ref
}

// selMsg è il messaggio selezionato completo (nil se il corpo non è ancora caricato).
func (m Model) selMsg() *msg.Message {
	if len(m.conv) == 0 {
		return nil
	}
	return m.conv[m.sel].full
}

// bodyHeight è l'altezza del viewport: header, allegati e command line escluse.
func (m Model) bodyHeight() int {
	used := headerLines
	if len(m.attLines) > 0 {
		used += 1 + len(m.attLines)
	}
	if m.cmdOpen {
		used++
	}
	return max(0, m.height-used)
}

// refresh ricalcola il pre-stilizzato dal modello: header e allegati del messaggio
// selezionato, e il contenuto del viewport (corpo singolo, oppure titoli e corpi
// espansi della conversazione con righe già a capo, così i titoli restano
// allineati a `starts` senza che il viewport ri-vada a capo).
func (m *Model) refresh() {
	m.cache.beginRound() // i corpi richiesti da questo giro (bodyOut) non si espellono
	m.header = m.renderHeader()
	m.attLines, m.attZones = nil, nil
	sel := m.selMsg()
	if sel != nil {
		att := m.renderAttachments(sel)
		m.attLines, m.attZones = att.Lines, att.Zones
	}
	if m.multi() {
		m.setContent(m.renderConversation())
		return
	}
	m.starts = nil
	var body render.Out
	if sel != nil {
		body = m.bodyOut(sel)
	}
	m.setContent(body, nil)
}

// setContent installa il contenuto nel viewport, che non va a capo da solo (il
// corpo e gia a capo dai renderer, cosi le zone restano sulla riga giusta), e le
// zone cliccabili per riga per il clic (Piano 15, Task 14). starts e la riga di
// inizio di ogni titolo della conversazione (nil nella vista singola).
func (m *Model) setContent(o render.Out, starts []int) {
	m.starts = starts
	m.linkRows = o.Zones
	m.vp.SetContent(strings.Join(o.Lines, "\n"))
}

// renderConversation compone il contenuto della conversazione e la riga di
// inizio di ogni titolo (indice nel contenuto, non sullo schermo). Un messaggio
// espanso senza corpo mostra un avviso al posto del testo. I corpi sono a blocchi
// (bodyOut): ricevitore puntatore perche possono accodare un rendering in
// background.
func (m *Model) renderConversation() (render.Out, []int) {
	var parts []render.Out
	starts := make([]int, len(m.conv))
	n := 0 // righe finora
	add := func(o render.Out) {
		parts = append(parts, o)
		n += len(o.Lines)
	}
	line := func(s string) render.Out { return render.Out{Lines: []string{s}, Zones: [][]render.Zone{nil}} }
	for i, e := range m.conv {
		starts[i] = n
		add(line(m.renderTitle(i)))
		if !e.expanded {
			continue
		}
		if e.full == nil {
			add(line(m.st.ListDate.Render("corpo in caricamento…")))
			add(line(""))
			continue
		}
		add(m.bodyOut(e.full))
		add(line(""))
	}
	return render.Stack(parts...), starts
}

// renderTitle è la riga di un messaggio della conversazione: marcatore ▸/▾,
// mittente e data. Troncata alla larghezza: occupa sempre una sola riga. Il titolo
// non riporta il corpo: può non essere ancora caricato (Piano 8, Task 13).
// Colori dagli slot del tema.
func (m Model) renderTitle(i int) string {
	e := m.conv[i]
	mark := "▸"
	if e.expanded {
		mark = "▾"
	}
	head := render.TruncCols(fmt.Sprintf("%s %s · %s", mark, e.ref.From, FormatDate(e.ref.Date)), m.width)
	style := m.st.ListNormal
	switch {
	case i == m.sel:
		style = m.st.ListSelected
	case e.ref.Unread:
		style = m.st.ListUnread
	}
	return style.Render(head)
}

// needBody chiede il corpo del messaggio i se non è ancora caricato (Piano 8,
// Task 13): la Cmd emette BodyRequested, che la radice soddisfa con FetchMessage.
func (m Model) needBody(i int) tea.Cmd {
	if i < 0 || i >= len(m.conv) || m.conv[i].full != nil {
		return nil
	}
	req := BodyRequested{ID: m.conv[i].ref.ID, Anchor: m.id}
	return func() tea.Msg { return req }
}

// move seleziona il messaggio precedente o successivo (d = ±1), porta il suo
// titolo in vista e chiede il corpo se manca (r/f/o/:save agiscono sul selezionato).
func (m *Model) move(d int) tea.Cmd {
	n := min(max(m.sel+d, 0), len(m.conv)-1)
	if n == m.sel {
		return nil
	}
	m.sel = n
	m.refresh()
	m.resize()
	m.reveal()
	return m.needBody(n)
}

// toggle seleziona il messaggio i, alterna la sua espansione e chiede il corpo se
// manca (Piano 8, Task 13).
func (m *Model) toggle(i int) tea.Cmd {
	if i < 0 || i >= len(m.conv) {
		return nil
	}
	m.sel = i
	m.conv[i].expanded = !m.conv[i].expanded
	m.refresh()
	m.resize()
	m.reveal()
	return m.needBody(i)
}

// reveal scorre il viewport perché il titolo del messaggio selezionato sia in
// cima alla vista, se non è già visibile.
func (m *Model) reveal() {
	top := m.starts[m.sel]
	off := m.vp.YOffset()
	if top < off || top >= off+max(1, m.bodyHeight()) {
		m.vp.SetYOffset(top)
	}
}

// ClickRow gestisce il clic sinistro alla riga y dello schermo (relativa alla vista
// thread): sul titolo di un messaggio della conversazione lo seleziona e ne alterna
// l'espansione. Ritorna false se la riga non è un titolo o non c'è conversazione;
// altrimenti la Cmd dell'eventuale richiesta del corpo.
func (m *Model) ClickRow(y int) (bool, tea.Cmd) {
	if !m.multi() || y < headerLines || y-headerLines >= m.bodyHeight() {
		return false, nil
	}
	row := y - headerLines + m.vp.YOffset()
	for i, s := range m.starts {
		if s == row {
			return true, m.toggle(i)
		}
	}
	return false, nil
}

// Refs elenca le righe dei messaggi della conversazione, in ordine (test e radice).
func (m Model) Refs() []msg.MessageRef {
	out := make([]msg.MessageRef, 0, len(m.conv))
	for _, e := range m.conv {
		out = append(out, e.ref)
	}
	return out
}
