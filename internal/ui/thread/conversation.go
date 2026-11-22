package thread

// Vista conversazione stile Gmail (Piano 8, Task 10): più messaggi nello stesso
// viewport, i letti compressi su una riga di titolo (mittente, data, anteprima),
// l'ultimo e i non letti espansi. Un solo messaggio resta la vista di sempre.

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

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

// selMsg è il messaggio selezionato completo (nil se il corpo non è caricato).
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
	m.header = m.renderHeader()
	m.attLines = nil
	sel := m.selMsg()
	if sel != nil {
		m.attLines = m.renderAttachments(sel)
	}
	if m.multi() {
		m.vp.SoftWrap = false
		content, starts := m.renderConversation()
		m.starts = starts
		m.vp.SetContent(content)
		return
	}
	m.vp.SoftWrap = true
	m.starts = nil
	if sel != nil {
		m.vp.SetContent(m.renderBody(sel))
	}
}

// renderConversation compone il contenuto della conversazione e la riga di
// inizio di ogni titolo (indice nel contenuto, non sullo schermo).
func (m Model) renderConversation() (string, []int) {
	var lines []string
	starts := make([]int, len(m.conv))
	for i, e := range m.conv {
		starts[i] = len(lines)
		lines = append(lines, m.renderTitle(i))
		if e.expanded && e.full != nil {
			lines = append(lines, wrapLines(m.renderBody(e.full), m.width)...)
			lines = append(lines, "")
		}
	}
	return strings.Join(lines, "\n"), starts
}

// renderTitle è la riga di un messaggio della conversazione: marcatore ▸/▾,
// mittente, data e, da compresso, la prima riga utile del corpo. Troncata alla
// larghezza: occupa sempre una sola riga. Colori dagli slot del tema.
func (m Model) renderTitle(i int) string {
	e := m.conv[i]
	mark := "▸"
	if e.expanded {
		mark = "▾"
	}
	head := fmt.Sprintf("%s %s · %s", mark, e.ref.From, FormatDate(e.ref.Date))
	if !e.expanded && e.full != nil {
		if s := snippet(e.full.BodyText); s != "" {
			head += " · " + s
		}
	}
	head = truncWidth(head, m.width)
	style := m.st.ListNormal
	switch {
	case i == m.sel:
		style = m.st.ListSelected
	case e.ref.Unread:
		style = m.st.ListUnread
	}
	return style.Render(head)
}

// snippet è la prima riga non vuota e non citata del corpo: l'anteprima del
// messaggio compresso.
func snippet(body string) string {
	for _, ln := range strings.Split(body, "\n") {
		ln = strings.TrimSpace(ln)
		if ln != "" && !strings.HasPrefix(ln, ">") {
			return ln
		}
	}
	return ""
}

// truncWidth tronca s alla larghezza w (con ellissi), contando le colonne visibili.
func truncWidth(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	var b strings.Builder
	cur := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if cur+rw > w-1 {
			break
		}
		b.WriteRune(r)
		cur += rw
	}
	return b.String() + "…"
}

// wrapLines spezza il testo a capo sulla larghezza w: ogni riga restituita occupa
// una sola riga del viewport (w <= 0: nessun a capo).
func wrapLines(s string, w int) []string {
	if w <= 0 {
		return strings.Split(s, "\n")
	}
	return strings.Split(lipgloss.NewStyle().Width(w).Render(s), "\n")
}

// move seleziona il messaggio precedente o successivo (d = ±1) e porta il suo
// titolo in vista.
func (m *Model) move(d int) {
	n := min(max(m.sel+d, 0), len(m.conv)-1)
	if n == m.sel {
		return
	}
	m.sel = n
	m.refresh()
	m.resize()
	m.reveal()
}

// toggle seleziona il messaggio i e alterna la sua espansione.
func (m *Model) toggle(i int) {
	if i < 0 || i >= len(m.conv) {
		return
	}
	m.sel = i
	m.conv[i].expanded = !m.conv[i].expanded
	m.refresh()
	m.resize()
	m.reveal()
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
// l'espansione. Ritorna false se la riga non è un titolo o non c'è conversazione.
func (m *Model) ClickRow(y int) bool {
	if !m.multi() || y < headerLines || y-headerLines >= m.bodyHeight() {
		return false
	}
	row := y - headerLines + m.vp.YOffset()
	for i, s := range m.starts {
		if s == row {
			m.toggle(i)
			return true
		}
	}
	return false
}
