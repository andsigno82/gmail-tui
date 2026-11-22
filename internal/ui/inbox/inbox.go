// Package inbox contiene la vista lista messaggi della TUI, costruita su
// bubbles/list v2 con un delegate custom: marker non letto, stella, mittente
// troncato, oggetto e data relativa in italiano. L'app radice delega tasti e
// mouse a questo modello quando la vista attiva è l'inbox.
package inbox

import (
	"fmt"
	"io"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/styles"
)

// DoubleClickWindow è l'intervallo massimo fra due click sulla stessa riga (o
// sull'anteprima) perché vengano interpretati come doppio click.
const DoubleClickWindow = 500 * time.Millisecond

// Larghezze del layout riga: il marker e la stella occupano due colonne da
// due celle, la data è allineata a destra, il mittente ha una colonna
// adattiva fra senderMin e senderMax, l'oggetto riempie il resto.
const (
	markerCol  = 4 // "● " + "★ "
	senderMin  = 10
	senderMax  = 24
	subjectMin = 3
)

// mesiIT sono i nomi dei mesi abbreviati usati dalla data relativa.
var mesiIT = [...]string{"gen", "feb", "mar", "apr", "mag", "giu", "lug", "ago", "set", "ott", "nov", "dic"}

// SelectedMsg notifica all'app l'apertura del messaggio (enter o doppio
// click): la radice passa a viewThread con il ref (Task 5 collega il fetch).
type SelectedMsg struct {
	Ref msg.MessageRef
}

// item adatta una riga-conversazione all'interfaccia list.Item di bubbles.
// ref è il messaggio più recente (Conversation.Latest): è il messaggio su cui
// si apre, si seleziona e si opera (Piano 8, Task 9).
type item struct {
	ref  msg.MessageRef
	conv mail.Conversation
}

// FilterValue implementa list.Item (il filtraggio è disabilitato, ma
// l'interfaccia lo richiede).
func (i item) FilterValue() string { return i.ref.From + " " + i.ref.Subject }

// Model è la vista inbox: la lista messaggi. Il render della riga è
// demandato a delegate; enter/doppio click producono SelectedMsg.
type Model struct {
	list  list.Model
	st    styles.Styles
	clock func() time.Time // date relative e doppio click (seam di test)

	// stato del click per il riconoscimento del doppio click.
	lastClick    time.Time
	lastClickIdx int
}

// New costruisce l'inbox sui ref dati (l'ordine di arrivo è preservato: lo
// store consegna già data desc, qui nessun riordino).
func New(refs []msg.MessageRef, st styles.Styles) Model {
	d := delegate{st: st, clock: time.Now}
	l := list.New(refsToItems(refs), d, 0, 0)
	// La radice possiede già titolo/status/paginazione/help: la lista rende
	// solo le righe dei messaggi, senza decorazioni bubbles.
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false)
	// Keymap: le lettere singole di default (h/l/u/d/b/f) tornano alla
	// keymap globale dell'app (Task 7 userà "l" per le label); la
	// paginazione resta su frecce e pgup/pgdn.
	km := list.DefaultKeyMap()
	km.PrevPage = key.NewBinding(key.WithKeys("left", "pgup"), key.WithHelp("←/pgup", "pag. prec."))
	km.NextPage = key.NewBinding(key.WithKeys("right", "pgdown"), key.WithHelp("→/pgdn", "pag. succ."))
	l.KeyMap = km
	return Model{list: l, st: st, clock: time.Now}
}

// Update gestisce i messaggi della vista: enter apre il messaggio
// selezionato (SelectedMsg), il mouse naviga (rotella e click — bubbles/list
// v2.2.2 non gestisce il mouse, handler custom), gli altri messaggi passano
// alla lista bubbles (j/k, frecce, paginazione, …).
func (m Model) Update(ev tea.Msg) (Model, tea.Cmd) {
	switch ev := ev.(type) {
	case tea.KeyPressMsg:
		if ev.String() == "enter" {
			return m, m.openSelected()
		}
	case tea.MouseClickMsg:
		cmd := m.click(ev)
		return m, cmd
	case tea.MouseWheelMsg:
		switch ev.Button {
		case tea.MouseWheelUp:
			m.list.CursorUp()
		case tea.MouseWheelDown:
			m.list.CursorDown()
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(ev)
	return m, cmd
}

// View rende la lista come stringa: le sub-viste v2 restano string, solo la
// radice le incapsula in tea.NewView.
func (m Model) View() string { return m.list.View() }

// SetRefs sostituisce i messaggi della lista preservando l'ordine di arrivo.
// Ricevitore puntatore: bubbles/list v2 muta lo stato interno con
// list.SetItems e un ricevitore valore perderebbe l'aggiornamento. La cmd
// restituita è quella di SetItems (nil col filtraggio disabilitato).
func (m *Model) SetRefs(refs []msg.MessageRef) tea.Cmd {
	return m.list.SetItems(refsToItems(refs))
}

// SetUnread aggiorna il marcatore di lettura di una riga senza rilistare (Piano
// 7.2, fix review): il segna-come-letto in anteprima si vede subito in lista.
func (m *Model) SetUnread(id msg.MessageID, unread bool) tea.Cmd {
	for i, it := range m.list.Items() {
		row, ok := it.(item)
		if ok && row.ref.ID == id {
			row.ref.Unread = unread
			// Il grassetto della conversazione segue il messaggio letto: se la
			// conversazione ha altri non letti, il prossimo refresh lo riallinea.
			row.conv.Unread = unread
			row.conv.Latest = row.ref
			return m.list.SetItem(i, row)
		}
	}
	return nil
}

// SetConversations sostituisce le righe con le conversazioni (Piano 8, Task 9):
// una riga per conversazione, con partecipanti, contatore, stato e label.
func (m *Model) SetConversations(convs []mail.Conversation) tea.Cmd {
	return m.list.SetItems(convsToItems(convs))
}

// SetSize propaga le dimensioni del corpo (l'app radice riserva l'ultima
// riga alla status bar) alla lista interna.
func (m *Model) SetSize(width, height int) { m.list.SetSize(width, height) }

// Select posiziona il cursore sull'indice globale dato (ripristino della
// selezione dopo un refresh e uso dai test).
func (m *Model) Select(i int) { m.list.Select(i) }

// AppendRefs accoda una pagina successiva alla lista (scroll in fondo, Piano 7,
// Task 3). Dedup per ID: posta arrivata fra due pagine, o un offset slittato,
// non duplica righe. Il cursore resta sulla riga su cui era.
func (m *Model) AppendRefs(refs []msg.MessageRef) tea.Cmd {
	items := m.list.Items()
	seen := make(map[msg.MessageID]bool, len(items)+len(refs))
	all := make([]list.Item, 0, len(items)+len(refs))
	for _, it := range items {
		if i, ok := it.(item); ok {
			seen[i.ref.ID] = true
			all = append(all, it)
		}
	}
	for _, r := range refs {
		if !seen[r.ID] {
			seen[r.ID] = true
			all = append(all, item{ref: r, conv: mail.SingleConversation(r)})
		}
	}
	idx := m.list.Index()
	cmd := m.list.SetItems(all)
	m.list.Select(idx)
	return cmd
}

// AppendConversations accoda una pagina di conversazioni (Piano 8, Task 9).
// Dedup per ID del messaggio più recente, come AppendRefs.
func (m *Model) AppendConversations(convs []mail.Conversation) tea.Cmd {
	items := m.list.Items()
	seen := make(map[msg.MessageID]bool, len(items)+len(convs))
	all := make([]list.Item, 0, len(items)+len(convs))
	for _, it := range items {
		if i, ok := it.(item); ok {
			seen[i.ref.ID] = true
			all = append(all, it)
		}
	}
	for _, c := range convs {
		if !seen[c.Latest.ID] {
			seen[c.Latest.ID] = true
			all = append(all, item{ref: c.Latest, conv: c})
		}
	}
	idx := m.list.Index()
	cmd := m.list.SetItems(all)
	m.list.Select(idx)
	return cmd
}

// AtEnd dice se il cursore è sull'ultima riga: è il segnale per chiedere la
// pagina successiva. Lista vuota: mai in fondo (nulla da scorrere).
func (m Model) AtEnd() bool {
	n := len(m.list.Items())
	return n > 0 && m.list.Index() == n-1
}

// SelectedRef restituisce il ref della riga sotto il cursore; serve all'app
// per le mutazioni sul messaggio selezionato (Task 7).
func (m Model) SelectedRef() (msg.MessageRef, bool) {
	if it, ok := m.list.SelectedItem().(item); ok {
		return it.ref, true
	}
	return msg.MessageRef{}, false
}

// SelectedIndex restituisce l'indice globale della riga sotto il cursore:
// l'app lo usa per ripristinare la posizione del cursore dopo il refresh
// silenzioso che segue una mutazione (Task 7).
func (m Model) SelectedIndex() int { return m.list.Index() }

// SetClock sostituisce l'orologio usato per date relative e doppio click:
// seam per test deterministici.
func (m *Model) SetClock(f func() time.Time) {
	m.clock = f
	m.list.SetDelegate(delegate{st: m.st, clock: f})
}

// SetStyles sostituisce gli stili del tema a runtime (cambio tema con `:theme`,
// Piano 4 Task 2): aggiorna la copia locale e ricostruisce il delegate, che
// custodisce gli stili per il render delle righe. Lista e cursore restano.
func (m *Model) SetStyles(st styles.Styles) {
	m.st = st
	m.list.SetDelegate(delegate{st: st, clock: m.clock})
}

// openSelected produce la Cmd che notifica il ref sotto il cursore.
func (m Model) openSelected() tea.Cmd {
	ref, ok := m.SelectedRef()
	if !ok {
		return nil
	}
	return func() tea.Msg { return SelectedMsg{Ref: ref} }
}

// click gestisce il click sinistro: seleziona la riga colpita; un secondo
// click sulla stessa riga entro doubleClickWindow apre il messaggio
// (SelectedMsg). La v2 di bubbletea non espone un contatore di click, quindi
// il doppio click è riconosciuto con l'orologio del modello. Ricevitore
// puntatore: la selezione e lo stato del click devono persistere nel modello.
func (m *Model) click(ev tea.MouseClickMsg) tea.Cmd {
	if ev.Button != tea.MouseLeft {
		return nil
	}
	idx, ok := m.rowAt(ev.Y)
	if !ok {
		return nil
	}
	m.list.Select(idx)
	now := m.clock()
	dbl := m.lastClickIdx == idx && now.Sub(m.lastClick) <= DoubleClickWindow
	m.lastClick, m.lastClickIdx = now, idx
	if dbl {
		return m.openSelected()
	}
	return nil
}

// rowAt converte la riga Y del terminale nell'indice globale della lista:
// senza decorazioni le righe partono dalla prima riga del corpo.
func (m Model) rowAt(y int) (int, bool) {
	if y < 0 {
		return 0, false
	}
	perPage := m.list.Paginator.PerPage
	if perPage > 0 && y >= perPage {
		return 0, false
	}
	idx := m.list.Paginator.Page*perPage + y
	if idx >= len(m.list.VisibleItems()) {
		return 0, false
	}
	return idx, true
}

// refsToItems converte i ref in item bubbles preservando l'ordine: ogni ref è
// una conversazione di un solo messaggio.
func refsToItems(refs []msg.MessageRef) []list.Item {
	items := make([]list.Item, len(refs))
	for i, r := range refs {
		items[i] = item{ref: r, conv: mail.SingleConversation(r)}
	}
	return items
}

// convsToItems converte le conversazioni in item bubbles preservando l'ordine.
func convsToItems(convs []mail.Conversation) []list.Item {
	items := make([]list.Item, len(convs))
	for i, c := range convs {
		items[i] = item{ref: c.Latest, conv: c}
	}
	return items
}

// RelativeDate formatta la data di un messaggio in forma compatta italiana,
// deterministica: stesso giorno → "08:15"; giorno precedente → "ieri";
// stesso anno → "15 nov"; anni diversi → "3 gen 25".
func RelativeDate(d, now time.Time) string {
	d = d.In(now.Location())
	if stessoGiorno(d, now) {
		return d.Format("15:04")
	}
	if stessoGiorno(d, now.AddDate(0, 0, -1)) {
		return "ieri"
	}
	mese := mesiIT[d.Month()-1]
	if d.Year() == now.Year() {
		return fmt.Sprintf("%d %s", d.Day(), mese)
	}
	return fmt.Sprintf("%d %s %02d", d.Day(), mese, d.Year()%100)
}

// stessoGiorno confronta le date di calendario (ignora ora e fuso orario).
func stessoGiorno(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// delegate è l'ItemDelegate custom: una riga per messaggio, stile per slot
// del tema (ListUnread/ListNormal/ListStarred/ListDate/ListSelected).
type delegate struct {
	st    styles.Styles
	clock func() time.Time
}

// Height implementa list.ItemDelegate: una riga per messaggio.
func (d delegate) Height() int { return 1 }

// Spacing implementa list.ItemDelegate: nessuna riga vuota fra le righe.
func (d delegate) Spacing() int { return 0 }

// Update implementa list.ItemDelegate: enter e mouse sono gestiti dal
// Model, niente update per item.
func (d delegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

// Render implementa list.ItemDelegate: compone la riga e la scrive su w.
func (d delegate) Render(w io.Writer, m list.Model, index int, li list.Item) {
	it, ok := li.(item)
	if !ok {
		return
	}
	fmt.Fprint(w, d.row(m.Width(), it.conv, index == m.Index()))
}

// row compone la riga della conversazione (Piano 8, Task 9): marker e grassetto
// seguono lo stato della conversazione (almeno un non letto), poi slot stella,
// partecipanti con contatore su colonna adattiva, oggetto del messaggio più
// recente, etichette utente e data relativa allineata a destra. Larghezza
// adattiva: la riga riempie esattamente la larghezza della lista.
func (d delegate) row(width int, conv mail.Conversation, selected bool) string {
	ref := conv.Latest
	marker := "○"
	if conv.Unread {
		marker = "●"
	}
	star := " "
	if ref.Starred {
		star = "★"
	}
	sender := senderLabel(conv)

	// Larghezza non ancora nota (nessun WindowSizeMsg): riga minima senza
	// truncation né data.
	if width <= markerCol {
		return fmt.Sprintf("%s %s %s %s", marker, star, sender, ref.Subject)
	}

	date := RelativeDate(ref.Date, d.clock())
	dateW := lipgloss.Width(date)

	// Colonne: mittente = frazione della larghezza con minimi/massimi,
	// oggetto = resto della riga fino alla data. Le etichette prendono al più
	// metà dello spazio dell'oggetto: quelle che non entrano diventano "+N".
	senderW := min(senderMax, max(senderMin, width/3))
	subjectW := max(subjectMin, width-dateW-markerCol-1-senderW-1)
	shown, more := fitChips(conv.Labels, subjectW/2)
	plainChips := d.renderChips(shown, more, true)
	chipsW := lipgloss.Width(plainChips)
	if chipsW > 0 {
		subjectW = max(subjectMin, subjectW-chipsW-1)
	}
	mid := padRight(truncate(sender, senderW), senderW) + " " + truncate(ref.Subject, subjectW)

	// La data è allineata a destra: riempi con spazi fino alla sua colonna.
	used := markerCol + lipgloss.Width(mid) + dateW
	if chipsW > 0 {
		used += 1 + chipsW
	}
	gap := max(0, width-used)

	if selected {
		chips := ""
		if chipsW > 0 {
			chips = " " + plainChips
		}
		return d.st.ListSelected.Render(marker + " " + star + " " + mid + chips +
			strings.Repeat(" ", gap) + date)
	}
	markerSt, bodySt := d.st.ListNormal, d.st.ListNormal
	if conv.Unread {
		markerSt, bodySt = d.st.ListUnread, d.st.ListUnread
	}
	starSt := d.st.ListNormal
	if ref.Starred {
		starSt = d.st.ListStarred
	}
	chips := ""
	if chipsW > 0 {
		chips = " " + d.renderChips(shown, more, false)
	}
	return markerSt.Render(marker) + starSt.Render(" "+star+" ") + bodySt.Render(mid) +
		chips + strings.Repeat(" ", gap) + d.st.ListDate.Render(date)
}

// senderLabel compone il nome dei partecipanti come Gmail: il primo, "e altri N"
// se sono di più, poi il contatore "(N)" se la conversazione ha più messaggi.
// Senza partecipanti ricade sul mittente del messaggio più recente.
func senderLabel(c mail.Conversation) string {
	name := c.Latest.From
	if n := len(c.Participants); n > 0 {
		name = c.Participants[0]
		if n > 1 {
			name += fmt.Sprintf(" e altri %d", n-1)
		}
	}
	if c.Count > 1 {
		name += fmt.Sprintf(" (%d)", c.Count)
	}
	return name
}

// chipText è il testo di una etichetta utente: il nome fra parentesi quadre.
func chipText(name string) string { return "[" + name + "]" }

// fitChips sceglie le etichette che entrano in budget colonne (Piano 8, Task 9):
// le prime che entrano, il resto è riassunto in "+N". Ritorna le etichette da
// mostrare e quante ne restano fuori; nessuna se nemmeno la prima entra.
func fitChips(labels []string, budget int) (shown []string, more int) {
	for n := len(labels); n > 0; n-- {
		w := 0
		for i, name := range labels[:n] {
			if i > 0 {
				w++
			}
			w += lipgloss.Width(chipText(name))
		}
		rest := len(labels) - n
		if rest > 0 {
			w += len(fmt.Sprintf(" +%d", rest))
		}
		if w <= budget {
			return labels[:n], rest
		}
	}
	return nil, 0
}

// renderChips compone le etichette mostrate separate da spazio, più il "+N".
// Ogni etichetta prende lo stile di Styles.LabelStyle; plain dà il solo testo
// (riga selezionata, dove lo stile è quello della selezione, e misura in colonne).
func (d delegate) renderChips(shown []string, more int, plain bool) string {
	parts := make([]string, 0, len(shown)+1)
	for _, name := range shown {
		if plain {
			parts = append(parts, chipText(name))
		} else {
			parts = append(parts, d.st.LabelStyle(name).Render(chipText(name)))
		}
	}
	if more > 0 {
		if plain {
			parts = append(parts, fmt.Sprintf("+%d", more))
		} else {
			parts = append(parts, d.st.ListDate.Render(fmt.Sprintf("+%d", more)))
		}
	}
	return strings.Join(parts, " ")
}

// truncate accorcia s a n rune aggiungendo "…"; n <= 0 rende stringa vuota.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return ""
	}
	return string(r[:n-1]) + "…"
}

// padRight allinea s alla larghezza n con spazi finali.
func padRight(s string, n int) string {
	if pad := n - lipgloss.Width(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}
