package app

// Vista conversazione nel thread (Piano 8, Task 10): caricamento dei messaggi
// della conversazione e segna-come-letto dei suoi non letti in una sola op.

import (
	"context"
	"sort"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// loadConversation restituisce i messaggi della conversazione di ref, in ordine
// cronologico e con il corpo. Senza X-GM-THRID (ThreadID 0, fallback senza
// X-GM-EXT-1) è il solo messaggio. Spam e cestino restano fuori (Q5/D8-7), salvo
// quando la cartella corrente è spam o cestino: allora si aggiungono i suoi membri.
// La riga aperta c'è sempre.
func loadConversation(ctx context.Context, st mail.MailStore, ref msg.MessageRef, folder mail.Mailbox) ([]*msg.Message, error) {
	if ref.ThreadID == 0 {
		mm, err := st.FetchMessage(ctx, ref.ID)
		if err != nil {
			return nil, err
		}
		return []*msg.Message{mm}, nil
	}
	refs, err := st.ThreadMessages(ctx, ref.ThreadID)
	if err != nil {
		return nil, err
	}
	var ids []msg.MessageID
	seen := make(map[msg.MessageID]bool)
	add := func(id msg.MessageID) {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, r := range refs {
		add(r.ID)
	}
	if folder.Role == "trash" || folder.Role == "junk" {
		members, err := st.ConversationMembers(ctx, folder.Name, ref.ThreadID)
		if err != nil {
			return nil, err
		}
		for _, id := range members {
			add(id)
		}
	}
	add(ref.ID)
	out := make([]*msg.Message, 0, len(ids))
	for _, id := range ids {
		mm, err := st.FetchMessage(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, mm)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Ref.Date.Before(out[j].Ref.Date) })
	return out, nil
}

// anchorMsg è il messaggio con ID id tra quelli caricati (nil se assente).
func anchorMsg(convs []*msg.Message, id msg.MessageID) *msg.Message {
	for _, mm := range convs {
		if mm.Ref.ID == id {
			return mm
		}
	}
	return nil
}

// loadedConv sceglie i messaggi da mostrare: la conversazione caricata, o il solo
// messaggio m se la conversazione non è arrivata (chiamanti e test a messaggio singolo).
func loadedConv(convs []*msg.Message, m *msg.Message) []*msg.Message {
	if len(convs) > 0 {
		return convs
	}
	return []*msg.Message{m}
}

// markReadIDs segna letti in una sola op i messaggi non letti ids della
// conversazione aperta dalla riga ref (Piano 8, Task 10). Errore solo in status.
// Con X-GM-THRID il riepilogo della riga si aggiorna con lo stato della
// conversazione (come markReadConv di Task 9b).
func markReadIDs(st mail.MailStore, ref msg.MessageRef, ids []msg.MessageID) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		err := st.MarkRead(ctx, ids, true)
		ev := opDone{label: "letta", err: err, refresh: false, localID: ref.ID, localRead: true}
		if err == nil && ref.ThreadID != 0 {
			if thread, terr := st.ThreadMessages(ctx, ref.ThreadID); terr == nil {
				ev.convSet, ev.convUnread = true, anyUnread(thread)
			}
		}
		return ev
	}
}
