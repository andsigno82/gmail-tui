// Azioni sulla selezione (Piano 10, Task 3, D10-S2/D10-S3/D10-V1): con righe
// marcate `a`, `#`, `s` e `v` agiscono su TUTTE le conversazioni selezionate,
// nella cartella corrente (conversationIDs e guardBox come per la singola, D10-T7).
// Dopo l'azione la selezione si azzera. Il risultato porta il listGen di partenza:
// se la lista cambia mentre l'op è in volo, l'esito è stante e non tocca la lista.
package app

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/toast"
)

// selectedConvs restituisce le conversazioni selezionate, nell'ordine della lista.
func (m Model) selectedConvs() []mail.Conversation {
	var out []mail.Conversation
	for _, c := range m.inbox.Conversations() {
		if m.sel[c.Latest.ID] {
			out = append(out, c)
		}
	}
	return out
}

// selectedRefs sono i messaggi più recenti delle conversazioni selezionate (la
// riga della lista), nell'ordine della lista.
func (m Model) selectedRefs() []msg.MessageRef {
	convs := m.selectedConvs()
	refs := make([]msg.MessageRef, len(convs))
	for i, c := range convs {
		refs[i] = c.Latest
	}
	return refs
}

// roleOfBox è il ruolo noto della mailbox box: quella mostrata o la posta in
// arrivo; altrove "" (ignoto), come rowRole (Piano 10, Task 3).
func (m Model) roleOfBox(box string) string {
	if strings.EqualFold(box, m.folder.Name) {
		return m.folder.Role
	}
	if strings.EqualFold(box, "INBOX") {
		return "inbox"
	}
	return ""
}

// bulkOp esegue a/s/v sulla selezione (Piano 10, Task 3). Rifiuti con toast e
// selezione intatta; altrimenti la selezione si azzera e l'op parte fuori dal loop.
func (m Model) bulkOp(k string) (Model, tea.Cmd) {
	rows := m.selectedRefs()
	if len(rows) == 0 {
		m.setSel(nil)
		return m, nil
	}
	convs := m.selectedConvs()
	gen := m.listGen
	switch k {
	case "a":
		for _, r := range rows {
			if !strings.EqualFold(boxOf(r), "INBOX") {
				cmd := m.toast.Push(toast.Info, "archivia è disponibile solo in Posta in arrivo")
				return m, cmd
			}
		}
		m.setSel(nil)
		m.pending++
		return m, tea.Batch(bulkArchive(m.store, rows, gen), m.spinner.Tick)
	case "s":
		// D10-V1 (Gmail): se almeno una è senza stella, la stella va a tutte.
		on := false
		for _, r := range rows {
			if !r.Starred {
				on = true
				break
			}
		}
		m.setSel(nil)
		m.pending++
		return m, tea.Batch(bulkStar(m.store, rows, on, gen), m.spinner.Tick)
	case "v":
		// D10-V1 (Gmail): se almeno una è non letta, tutte lette; altrimenti tutte non lette.
		read := false
		for _, c := range convs {
			if c.Unread {
				read = true
				break
			}
		}
		m.setSel(nil)
		m.pending++
		return m, tea.Batch(bulkRead(m.store, rows, read, gen), m.spinner.Tick)
	}
	return m, nil
}

// askBulkTrash apre la conferma del cestino sulla selezione (`#`, Piano 10, Task 3).
// Nella cartella Cestino la selezione è rifiutata, come la singola riga.
func (m Model) askBulkTrash() (Model, tea.Cmd) {
	rows := m.selectedRefs()
	for _, r := range rows {
		if m.roleOfBox(boxOf(r)) == "trash" {
			cmd := m.toast.Push(toast.Info, "già nel cestino")
			return m, cmd
		}
	}
	if len(rows) == 0 {
		return m, nil
	}
	m.trashSel = rows
	m.trashOpen = true
	return m, nil
}

// bulkIDs risolve gli ID su cui agisce un'op sulla selezione: per ogni riga la
// conversazione nella sua mailbox (conversationIDs), guardata sulla mailbox della
// riga. Un solo rifiuto di guardia blocca tutta l'op. Senza duplicati.
func bulkIDs(ctx context.Context, st mail.MailStore, rows []msg.MessageRef) ([]msg.MessageID, error) {
	var out []msg.MessageID
	seen := make(map[msg.MessageID]bool)
	for _, r := range rows {
		ids, err := conversationIDs(ctx, st, r)
		if err == nil {
			err = guardBox(ids, boxOf(r))
		}
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out, nil
}

// bulkArchive archivia la conversazione di ogni riga (Piano 10, Task 3): feedback
// "archiviate N", refresh profondo come la singola.
func bulkArchive(st mail.MailStore, rows []msg.MessageRef, gen int) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		ids, err := bulkIDs(ctx, st, rows)
		if err == nil {
			err = st.Archive(ctx, ids)
		}
		return opDone{label: fmt.Sprintf("archiviate %d", len(rows)), err: err, refresh: true, deep: true, gone: ids, gen: gen, checkGen: true}
	}
}

// bulkTrash sposta nel cestino la conversazione di ogni riga (Piano 10, Task 3).
func bulkTrash(st mail.MailStore, rows []msg.MessageRef, gen int) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		ids, err := bulkIDs(ctx, st, rows)
		if err == nil {
			_, err = st.Trash(ctx, ids)
		}
		return opDone{label: fmt.Sprintf("spostate %d nel cestino", len(rows)), err: err, refresh: true, deep: true, gone: ids, gen: gen, checkGen: true}
	}
}

// bulkStar imposta la stella sul messaggio più recente di ogni riga, come la
// singola (Piano 8, Task 9b: la stella resta sul Latest).
func bulkStar(st mail.MailStore, rows []msg.MessageRef, on bool, gen int) tea.Cmd {
	return func() tea.Msg {
		state := "off"
		if on {
			state = "on"
		}
		err := st.Star(context.Background(), refIDs(rows), on)
		return opDone{label: fmt.Sprintf("stella %s su %d", state, len(rows)), err: err, refresh: true, gen: gen, checkGen: true}
	}
}

// bulkRead segna lette (o non lette) le conversazioni delle righe, tutto il thread
// in tutte le mailbox come la singola (markReadConv, D8-6).
func bulkRead(st mail.MailStore, rows []msg.MessageRef, read bool, gen int) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		var ids []msg.MessageID
		seen := make(map[msg.MessageID]bool)
		var err error
		for _, r := range rows {
			part := []msg.MessageID{r.ID}
			if r.ThreadID != 0 {
				var thread []msg.MessageRef
				thread, err = st.ThreadMessages(ctx, r.ThreadID)
				if err != nil {
					break
				}
				part = withID(refIDs(thread), r.ID)
			}
			for _, id := range part {
				if !seen[id] {
					seen[id] = true
					ids = append(ids, id)
				}
			}
		}
		if err == nil {
			err = st.MarkRead(ctx, ids, read)
		}
		label := fmt.Sprintf("%d segnate come non lette", len(rows))
		if read {
			label = fmt.Sprintf("%d segnate come lette", len(rows))
		}
		return opDone{label: label, err: err, refresh: true, gen: gen, checkGen: true}
	}
}
