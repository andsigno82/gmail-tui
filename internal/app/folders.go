// Selettore cartelle e label (Piano 7, Task 4, tasto `m`): una modale con
// bubbles/list sopra il corpo, come il help overlay. L'elenco viene da
// MailStore.ListMailboxes (ruoli SPECIAL-USE, mai nomi localizzati). La
// cartella corrente vive in Model.folder, separata dalla query di ricerca.
package app

import (
	"context"
	"fmt"
	"io"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/styles"
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

// folderItem adatta una mailbox all'interfaccia list.Item del selettore.
type folderItem struct {
	box mail.Mailbox
}

// FilterValue implementa list.Item (il filtraggio del selettore è disabilitato).
func (i folderItem) FilterValue() string { return folderName(i.box) }

// folderDelegate rende una riga per cartella con gli slot della lista del tema.
type folderDelegate struct {
	st styles.Styles
}

// Height implementa list.ItemDelegate: una riga per cartella.
func (d folderDelegate) Height() int { return 1 }

// Spacing implementa list.ItemDelegate: nessuna riga vuota fra le righe.
func (d folderDelegate) Spacing() int { return 0 }

// Update implementa list.ItemDelegate: nessun update per item.
func (d folderDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

// Render implementa list.ItemDelegate: marcatore sul cursore, nome della cartella.
func (d folderDelegate) Render(w io.Writer, m list.Model, index int, li list.Item) {
	it, ok := li.(folderItem)
	if !ok {
		return
	}
	name := folderName(it.box)
	if index == m.Index() {
		fmt.Fprint(w, d.st.ListSelected.Render("▸ "+name))
		return
	}
	fmt.Fprint(w, d.st.ListNormal.Render("  "+name))
}

// newFolderList costruisce la lista del selettore senza decorazioni bubbles (le
// stringhe e i colori di bubbles non sono del tema): titolo e help li rende
// foldersView.
func newFolderList(st styles.Styles) list.Model {
	l := list.New(nil, folderDelegate{st: st}, 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false)
	return l
}

// listMailboxes esegue ListMailboxes fuori dal loop UI (risultato: mailboxesLoaded).
func listMailboxes(st mail.MailStore) tea.Cmd {
	return func() tea.Msg {
		boxes, err := st.ListMailboxes(context.Background())
		return mailboxesLoaded{boxes: boxes, err: err}
	}
}

// openFolders apre il selettore (`m`): la lista arriva con ListMailboxes; fino
// ad allora la modale mostra il caricamento.
func (m Model) openFolders() (Model, tea.Cmd) {
	m.folderOpen = true
	m.pending++
	return m, tea.Batch(listMailboxes(m.store), m.spinner.Tick)
}

// chooseFolder chiude il selettore e passa alla cartella sotto il cursore. La
// stessa cartella chiude e basta: nessuna rilettura.
func (m Model) chooseFolder() (Model, tea.Cmd) {
	m.folderOpen = false
	it, ok := m.folderList.SelectedItem().(folderItem)
	if !ok {
		return m, nil
	}
	box := canonicalFolder(it.box)
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
	m.inbox.Select(0)
	m.status = statusMsg{text: "cartella " + folderName(box)}
	m.startup = true
	m.pending++
	return m, tea.Batch(loadRefs(m.store, box.Name), m.spinner.Tick)
}

// foldersView rende il corpo del selettore: titolo con l'hint dei tasti e la
// lista delle cartelle. Finché ListMailboxes non risponde (lista vuota) mostra
// il caricamento.
func (m Model) foldersView() string {
	if len(m.folderList.Items()) == 0 {
		return m.styles.ListDate.Render("caricamento cartelle…")
	}
	title := m.styles.ThreadHeader.Render("cartelle e label") +
		m.styles.ListDate.Render("  invio apre · esc chiude")
	return title + "\n" + m.folderList.View()
}
