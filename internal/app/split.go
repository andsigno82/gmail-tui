// Vista a due colonne (Piano 7.2, Task 4): lista messaggi a sinistra, anteprima
// del messaggio sotto il cursore a destra (riuso di thread.Model). Attiva solo
// con layout = split e larghezza ≥ 120 colonne (U4, U7): sotto, una colonna.
// Proporzione fissa: lista floor(35%), anteprima il resto meno il separatore
// (U8). Il focus (Tab, U6) dice quale colonna riceve j/k/pgup/pgdn; le azioni
// (r f a s v l # o :) agiscono sempre sul messaggio in anteprima. Il caricamento
// dell'anteprima aspetta 150 ms di cursore fermo; il segna-come-letto segue
// l'impostazione preview_mark_read (U5). Ogni token stantio è scartato.
package app

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/andsigno82/gmail-tui/internal/config"
	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/inbox"
)

// Parametri della vista a due colonne (Piano 7.2, Task 4).
const (
	splitMinWidth        = 120                    // sotto questa larghezza: una colonna (U4)
	splitListPercent     = 35                     // quota della lista in percentuale (U8)
	splitSepWidth        = 1                      // colonne del separatore verticale
	previewLoadDelay     = 150 * time.Millisecond // cursore fermo prima del fetch dell'anteprima
	previewMarkReadDelay = 2 * time.Second        // "dopo 2 secondi" (U5)
	splitPreviewPad      = 1                      // colonne di padding a sinistra dell'anteprima (Piano 7.2, fix review)
)

// splitOn dice se la vista a due colonne è attiva: layout split e larghezza
// sufficiente (la soglia è quella di U4).
func (m Model) splitOn() bool {
	return m.layout == config.LayoutSplit && m.width >= splitMinWidth
}

// splitWidths restituisce le colonne di lista e anteprima per una larghezza
// data (il separatore è escluso da entrambe).
func splitWidths(width int) (list, preview int) {
	list = width * splitListPercent / 100
	preview = max(0, width-list-splitSepWidth)
	return list, preview
}

// previewFocused dice se il focus è sull'anteprima (Tab): solo nella vista
// inbox a due colonne.
func (m Model) previewFocused() bool {
	return m.focusPreview && m.splitOn() && m.view == viewInbox
}

// previewVisible dice se l'anteprima è in uso: vista inbox a due colonne con un
// messaggio sotto il cursore.
func (m Model) previewVisible() bool {
	return m.splitOn() && m.view == viewInbox && m.prevHas
}

// previewReady dice se il thread mostra il corpo del messaggio in anteprima.
func (m Model) previewReady() bool {
	return m.prevHas && m.thread.Loaded() && m.thread.AnchorID() == m.prevRef.ID
}

// threadTarget dice se i tasti del thread (command line, filepicker, scorrimento)
// vanno al thread: vista thread a schermo intero, o anteprima con il focus.
func (m Model) threadTarget() bool {
	return m.view == viewThread || (m.previewFocused() && m.previewReady())
}

// previewRef è il messaggio dell'anteprima: la copia del thread quando è
// caricata (stato aggiornato da MarkRead e op), altrimenti la riga del cursore.
func (m Model) previewRef() (msg.MessageRef, bool) {
	if !m.prevHas {
		return msg.MessageRef{}, false
	}
	if m.previewReady() {
		return m.thread.Ref(), true
	}
	return m.prevRef, true
}

// relayout riapplica le dimensioni alle viste dopo un cambio di misura, layout o
// vista. Fuori dal layout a due colonne il focus e l'anteprima si spengono: al
// ritorno l'anteprima si ricarica dal cursore.
func (m *Model) relayout() {
	if m.width == 0 {
		return
	}
	if !m.splitOn() {
		m.focusPreview = false
		m.prevHas = false
		m.prevTok++
	}
	m.sizeInbox()
	m.sizeThread()
}

// sizeInbox dà alla lista la colonna sinistra (due colonne) o tutta la larghezza.
func (m *Model) sizeInbox() {
	h := max(0, m.height-1)
	if m.splitOn() {
		list, _ := splitWidths(m.width)
		m.inbox.SetSize(list, h)
		return
	}
	m.inbox.SetSize(m.width, h)
}

// sizeThread dà al thread l'intera vista se è aperto a schermo intero, altrimenti
// la colonna dell'anteprima (due colonne).
func (m *Model) sizeThread() {
	h := max(0, m.height-1)
	if m.view != viewThread && m.splitOn() {
		_, preview := splitWidths(m.width)
		m.thread.SetSize(max(0, preview-splitPreviewPad), h)
		return
	}
	m.thread.SetSize(m.width, h)
}

// syncPreview allinea l'anteprima al cursore: ogni cambio del messaggio sotto il
// cursore (movimento, refresh, cambio cartella o ricerca) apre un nuovo token e
// il timer di caricamento. Lista vuota → anteprima vuota. Fuori dalla vista
// inbox a due colonne non fa nulla.
func (m Model) syncPreview() (Model, tea.Cmd) {
	if !m.splitOn() || m.view != viewInbox {
		return m, nil
	}
	cur, ok := m.inbox.SelectedRef()
	if !ok {
		if m.prevHas {
			m.prevHas, m.prevRef = false, msg.MessageRef{}
			m.prevTok++
		}
		return m, nil
	}
	if m.prevHas && m.prevRef.ID == cur.ID {
		return m, nil
	}
	m.prevTok++
	m.prevRef, m.prevHas = cur, true
	return m, previewTimer(m.prevTok)
}

// previewTimer è il ritardo di 150 ms prima del fetch dell'anteprima.
func previewTimer(tok int) tea.Cmd {
	return tea.Tick(previewLoadDelay, func(time.Time) tea.Msg { return previewDue{tok: tok} })
}

// previewReadTimer è il ritardo di 2 s prima di segnare come letto l'anteprima.
func previewReadTimer(tok int, id msg.MessageID) tea.Cmd {
	return tea.Tick(previewMarkReadDelay, func(time.Time) tea.Msg {
		return previewReadDue{tok: tok, id: id}
	})
}

// fetchPreview esegue FetchMessage fuori dal loop UI (cache-first) per
// l'anteprima; il risultato porta token e ID per lo scarto degli stantii.
func fetchPreview(st mail.MailStore, tok int, ref msg.MessageRef, folder mail.Mailbox) tea.Cmd {
	return func() tea.Msg {
		refs, bodies, err := loadConversation(context.Background(), st, ref, folder)
		return previewLoaded{tok: tok, id: ref.ID, m: bodies[ref.ID], refs: refs, bodies: bodies, err: err}
	}
}

// previewMarkRead applica il segna-come-letto all'anteprima appena caricata (U5):
// subito → ora (con la stessa cmd del toggle senza refresh); dopo 2 s → timer
// con token; mai → nulla. Solo messaggi non letti.
func (m Model) previewMarkRead(tok int) (Model, tea.Cmd) {
	// Piano 8, Task 10: vale per tutti i non letti della conversazione in anteprima.
	unread := m.thread.UnreadIDs()
	if len(unread) == 0 {
		return m, nil
	}
	switch m.markRead {
	case config.PreviewMarkReadImmediate:
		m.thread.MarkRead()
		return m, markReadIDs(m.store, m.prevRef, unread)
	case config.PreviewMarkReadNever:
		return m, nil
	default: // delay (valore di default)
		return m, previewReadTimer(tok, m.thread.AnchorID())
	}
}

// previewView rende la colonna dell'anteprima alla larghezza e altezza date:
// il thread se il corpo è caricato, un testo attenuato altrimenti. Le righe
// troppo lunghe si troncano (nessun a capo che sposti la status bar).
func (m Model) previewView(w, h int) string {
	var body string
	switch {
	case !m.prevHas:
		body = m.styles.ListDate.Render("nessun messaggio")
	case m.previewReady():
		body = m.thread.View()
	default:
		// Caricamento: spinner e testo al centro orizzontale e verticale del
		// pannello (richiesta di Andrea, 2026-11-23). Lo spinner gira finché il
		// fetch è in volo (pending, previewDue arma il tick).
		wait := m.spinner.View() + " " + m.styles.ListDate.Render("Caricamento messaggio…")
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, wait)
	}
	return lipgloss.NewStyle().PaddingLeft(splitPreviewPad).MaxWidth(w).Height(h).MaxHeight(h).Render(body)
}

// splitView compone le due colonne con il separatore verticale: il colore del
// separatore indica il focus (slot primary sull'anteprima attiva).
func (m Model) splitView() string {
	list, preview := splitWidths(m.width)
	h := max(0, m.height-1)
	left := lipgloss.NewStyle().MaxWidth(list).Height(h).MaxHeight(h).Render(m.inbox.View())
	sepStyle := m.styles.Divider
	if m.previewFocused() {
		sepStyle = m.styles.DividerFocus
	}
	sep := sepStyle.Render(strings.TrimSuffix(strings.Repeat("│\n", h), "\n"))
	return lipgloss.JoinHorizontal(lipgloss.Top, left, sep, m.previewView(preview, h))
}

// previewClick gestisce il click sinistro sulla colonna dell'anteprima (Piano
// 7.2, fix review): dà il focus all'anteprima; un secondo click entro la finestra
// del doppio click apre il messaggio a schermo intero (come invio).
func (m Model) previewClick(y int) (Model, tea.Cmd) {
	m.focusPreview = true
	// Piano 8, Task 10: il click su un titolo della conversazione (riga y) la
	// espande o comprime; il doppio click la apre a schermo intero.
	if m.previewReady() {
		if ok, cmd := m.thread.ClickRow(y); ok {
			return m, cmd
		}
	}
	now := time.Now()
	dbl := !m.lastPreviewClick.IsZero() && now.Sub(m.lastPreviewClick) <= inbox.DoubleClickWindow
	if !dbl || !m.previewReady() {
		m.lastPreviewClick = now
		return m, nil
	}
	m.lastPreviewClick = time.Time{}
	ref := m.prevRef
	return m, func() tea.Msg { return inbox.SelectedMsg{Ref: ref} }
}

// setLocalRead aggiorna subito il marcatore di lettura della riga in lista e del
// ref in m.refs dopo un MarkRead riuscito (Piano 7.2, fix review). id vuoto = nessun
// cambio.
func (m *Model) setLocalRead(id msg.MessageID, read bool) tea.Cmd {
	if id == "" {
		return nil
	}
	for i := range m.refs {
		if m.refs[i].ID == id {
			m.refs[i].Unread = !read
		}
	}
	if c, ok := m.sums[id]; ok {
		c.Unread, c.Latest.Unread = !read, !read
		m.sums[id] = c
	}
	return m.inbox.SetUnread(id, !read)
}

// setConvUnread aggiorna lo stato di lettura dell'intera conversazione della riga
// id (Piano 8, Task 9b): riepilogo per ID e grassetto in lista, subito.
func (m *Model) setConvUnread(id msg.MessageID, unread bool) tea.Cmd {
	if c, ok := m.sums[id]; ok {
		c.Unread = unread
		m.sums[id] = c
	}
	return m.inbox.SetConvUnread(id, unread)
}

// mouseX è la colonna del terminale di un evento mouse (0 = bordo sinistro).
func mouseX(ev tea.Msg) int {
	if mm, ok := ev.(tea.MouseMsg); ok {
		return mm.Mouse().X
	}
	return 0
}
