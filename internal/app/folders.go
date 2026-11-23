// Selettore cartelle e label (Piano 7, Task 4, tasto `m`): una modale con
// l'albero di internal/ui/foldertree (Piano 15, Task 15, D15-09: label annidate
// sul Delimiter, contatori dei non letti) sopra il corpo, come il help overlay.
// Il selettore come finestra sovrapposta e del Task 16. L'elenco viene da
// MailStore.ListMailboxes (ruoli SPECIAL-USE, mai nomi localizzati). La
// cartella corrente vive in Model.folder, separata dalla query di ricerca.
package app

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2/tree"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/foldertree"
)

// mailboxesLoaded è il risultato di ListMailboxes per il selettore (Piano 7,
// Task 4). Chiuso il selettore prima della risposta, la risposta è ignorata.
type mailboxesLoaded struct {
	boxes []mail.Mailbox
	err   error
}

// folderName è l'etichetta leggibile di una cartella: ruoli tradotti in
// italiano, label e cartelle senza ruolo col nome IMAP.
func folderName(b mail.Mailbox) string {
	switch b.Role {
	case "inbox":
		return "Posta in arrivo"
	case "sent":
		return "Inviati"
	case "drafts":
		return "Bozze"
	case "trash":
		return "Cestino"
	case "junk":
		return "Spam"
	case "all":
		return "Tutta la posta"
	case "flagged":
		return "Speciali"
	}
	return b.Name
}

// canonicalFolder riduce la posta in arrivo alla cartella zero (Name ""): è la
// Query.Label vuota del contratto (INBOX). Le altre cartelle restano com'è.
func canonicalFolder(b mail.Mailbox) mail.Mailbox {
	if b.Role == "inbox" || b.Name == "INBOX" {
		return mail.Mailbox{}
	}
	return b
}

// inFolder applica la cartella corrente a una query di ricerca che non ne
// nomina una (label: esplicita vince). Piano 7, Task 4.
func inFolder(q msg.Query, box string) msg.Query {
	if q.Label == "" {
		q.Label = box
	}
	return q
}

// folderPicker e lo stato del selettore `m` (Piano 15, Task 15): l'albero di
// foldertree, le sue righe selezionabili (in ordine di albero), il cursore come
// indice in rows e gli ultimi contatori dei non letti. E un valore dentro il
// Model: i metodi con ricevitore puntatore agiscono sulla copia del chiamante,
// che poi ritorna il Model (gotcha del value receiver, AGENTS.md).
type folderPicker struct {
	boxes  []mail.Mailbox
	tree   *tree.Tree
	rows   []foldertree.Row
	cursor int
	counts map[string]int
}

// reset svuota l'albero (non i contatori: restano visibili fino al ricaricamento).
func (p *folderPicker) reset() {
	p.boxes, p.tree, p.rows, p.cursor = nil, nil, nil, 0
}

// load costruisce l'albero da boxes e porta il cursore sulla cartella corrente.
func (p *folderPicker) load(boxes []mail.Mailbox, current mail.Mailbox) {
	p.boxes = boxes
	p.tree = foldertree.Build(boxes, folderName)
	p.rows = foldertree.Rows(p.tree)
	p.cursor = 0
	for i, r := range p.rows {
		if b, ok := p.box(r.Name); ok && canonicalFolder(b) == current {
			p.cursor = i
			return
		}
	}
}

// box ritrova la mailbox dal nome IMAP.
func (p folderPicker) box(name string) (mail.Mailbox, bool) {
	for _, b := range p.boxes {
		if b.Name == name {
			return b, true
		}
	}
	return mail.Mailbox{}, false
}

// move sposta il cursore di d righe, senza uscire dall'albero (nessun giro).
func (p *folderPicker) move(d int) {
	if len(p.rows) == 0 {
		return
	}
	p.cursor = min(max(p.cursor+d, 0), len(p.rows)-1)
}

// selected e la mailbox sotto il cursore.
func (p folderPicker) selected() (mail.Mailbox, bool) {
	if p.cursor < 0 || p.cursor >= len(p.rows) {
		return mail.Mailbox{}, false
	}
	return p.box(p.rows[p.cursor].Name)
}

// listMailboxes esegue ListMailboxes fuori dal loop UI (risultato: mailboxesLoaded).
func listMailboxes(st mail.MailStore) tea.Cmd {
	return func() tea.Msg {
		boxes, err := st.ListMailboxes(context.Background())
		return mailboxesLoaded{boxes: boxes, err: err}
	}
}

// openFolders apre il selettore (`m`): la lista arriva con ListMailboxes; fino
// ad allora la modale mostra il caricamento. I contatori dei non letti partono
// insieme (Piano 15, Task 15), fuori dal loop UI e senza spinner: se falliscono
// il selettore resta senza contatori.
func (m Model) openFolders() (Model, tea.Cmd) {
	m.folderOpen = true
	m.folderPick.reset()
	m.pending++
	unread := m.requestUnread()
	return m, tea.Batch(listMailboxes(m.store), m.spinner.Tick, unread)
}

// chooseFolder chiude il selettore e passa alla cartella sotto il cursore. La
// stessa cartella chiude e basta: nessuna rilettura.
func (m Model) chooseFolder() (Model, tea.Cmd) {
	m.folderOpen = false
	sel, ok := m.folderPick.selected()
	if !ok {
		return m, nil
	}
	box := canonicalFolder(sel)
	if box == m.folder {
		return m, nil
	}
	return m.switchFolder(box)
}

// switchFolder porta la vista sulla cartella data (Piano 7, Task 4): una
// ricerca attiva si azzera (D6), lo scroll riparte da zero, il cursore torna in
// cima e la lettura è NON silenziosa. Il nome della cartella resta in status
// come il banner d'avvio (startup) finché il load non lo rimpiazza. Un refresh
// o una pagina in volo della cartella precedente si scartano al loro arrivo.
func (m Model) switchFolder(box mail.Mailbox) (Model, tea.Cmd) {
	m.folder = box
	m.query = msg.Query{}
	m.queryRaw = ""
	m.exhausted = false
	m.loadingMore = false
	m.dirty, m.dirtyDeep = false, false
	// Piano 7, fix code-review 4: le righe della cartella precedente non restano
	// visibili né azionabili finché il load non torna (lo stato vuoto è coerente).
	m.refs = nil
	clearRows := m.inbox.SetRefs(nil)
	m.inbox.Select(0)
	// Il nome della cartella è già nel tag della status bar: lo status si
	// svuota (fix review 6, niente "[Inviati] cartella Inviati").
	m.status = statusMsg{}
	m.startup = true
	m.pending++
	m.listGen++   // nuova generazione: le pagine e i refresh della cartella precedente sono stanti (fix review 2)
	m.setSel(nil) // Piano 10, Task 2 (D10-S3): la selezione non passa alla cartella nuova
	return m, tea.Batch(clearRows, loadRefs(m.store, box.Name, m.listGen), m.spinner.Tick)
}

// foldersView rende il corpo del selettore: titolo con l'hint dei tasti e
// l'albero delle cartelle (foldertree.Render, una riga per nodo). Finché
// ListMailboxes non risponde (nessuna riga) mostra il caricamento. Se l'albero è
// più alto dello spazio, si scorre per tenere visibile la riga del cursore.
func (m Model) foldersView() string {
	p := m.folderPick
	if len(p.rows) == 0 {
		return m.windowFor(m.styles.ListDate.Render("caricamento cartelle…"), "cartelle e label", folderMinW)
	}
	out, rows := foldertree.Render(p.tree, p.cursor, p.counts, m.styles)
	lines := strings.Split(out, "\n")
	// Piano 15, Task 16 (D15-11): finestra con ombra; righe visibili = corpo meno ombra,
	// bordo, titolo e riga dell'hint.
	if visible := m.height - 6; m.height > 0 && len(lines) > max(visible, 3) {
		visible = max(visible, 3)
		off := 0
		if p.cursor < len(rows) && rows[p.cursor].Line >= visible {
			off = rows[p.cursor].Line - visible + 1
		}
		lines = lines[off : off+visible]
	}
	hint := m.styles.ListDate.Render("invio apre · esc chiude")
	return m.windowFor(strings.Join(lines, "\n")+"\n"+hint, "cartelle e label", folderMinW)
}

// folderMinW e la larghezza interna minima della finestra del selettore cartelle.
const folderMinW = 36
