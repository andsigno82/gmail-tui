package app

// Vista conversazione nel thread (Piano 8, Task 10): caricamento dei messaggi
// della conversazione e segna-come-letto dei suoi non letti in una sola op.

import (
	"context"
	"fmt"
	"sort"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/thread"
)

// loadConversation restituisce le righe della conversazione di ref in ordine
// cronologico e i corpi dei soli messaggi espansi all'apertura (ultimo e non letti,
// Piano 8, Task 13): gli altri si scaricano alla selezione o all'espansione. Senza
// X-GM-THRID (ThreadID 0, fallback senza X-GM-EXT-1) è il solo messaggio. Spam e
// cestino restano fuori (Q5/D8-7), salvo quando la cartella corrente è spam o
// cestino: allora si aggiungono i suoi membri. La riga aperta c'è sempre.
func loadConversation(ctx context.Context, st mail.MailStore, ref msg.MessageRef, folder mail.Mailbox) ([]msg.MessageRef, map[msg.MessageID]*msg.Message, error) {
	if ref.ThreadID == 0 {
		mm, err := st.FetchMessage(ctx, ref.ID)
		if err != nil {
			return nil, nil, err
		}
		return []msg.MessageRef{mm.Ref}, map[msg.MessageID]*msg.Message{mm.Ref.ID: mm}, nil
	}
	threadRefs, err := st.ThreadMessages(ctx, ref.ThreadID)
	if err != nil {
		return nil, nil, err
	}
	bodies := make(map[msg.MessageID]*msg.Message)
	// Un messaggio Gmail sta in più mailbox con ID diversi (INBOX e Tutti i messaggi):
	// la deduplica è per GmailID, non per ID (Piano 8, review punto 2).
	have := make(map[string]bool, len(threadRefs)+1)
	haveID := make(map[msg.MessageID]bool, len(threadRefs)+1)
	all := make([]msg.MessageRef, 0, len(threadRefs)+1)
	for _, r := range threadRefs {
		if !have[copyKey(r)] {
			have[copyKey(r)] = true
			haveID[r.ID] = true
			all = append(all, r)
		}
	}
	// Membri di spam/cestino e riga aperta non sono in ThreadMessages: servono
	// le loro righe, quindi il loro FetchMessage (il corpo resta in bodies).
	var extra []msg.MessageID
	if folder.Role == "trash" || folder.Role == "junk" {
		members, err := st.ConversationMembers(ctx, folder.Name, ref.ThreadID)
		if err != nil {
			return nil, nil, err
		}
		extra = append(extra, members...)
	}
	extra = append(extra, ref.ID)
	for _, id := range extra {
		if haveID[id] {
			continue
		}
		haveID[id] = true
		mm, err := st.FetchMessage(ctx, id)
		if err != nil {
			return nil, nil, err
		}
		if have[copyKey(mm.Ref)] {
			continue // la stessa copia è già in lista con l'ID di un'altra mailbox
		}
		have[copyKey(mm.Ref)] = true
		bodies[id] = mm
		all = append(all, mm.Ref)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Date.Before(all[j].Date) })
	for _, id := range thread.DefaultExpanded(all) {
		if bodies[id] != nil {
			continue
		}
		mm, err := st.FetchMessage(ctx, id)
		if err != nil {
			return nil, nil, err
		}
		bodies[id] = mm
	}
	return all, bodies, nil
}

// copyKey identifica il messaggio: il GmailID (uguale in ogni mailbox), o l'ID se
// il messaggio non ha ID Gmail (non Gmail, fallback).
func copyKey(r msg.MessageRef) string {
	if r.GmailID != 0 {
		return fmt.Sprintf("g%d", r.GmailID)
	}
	return "i" + string(r.ID)
}

// loadedThread ricava righe e corpi da un risultato di caricamento: m è il solo
// messaggio dei chiamanti a messaggio singolo (test e fallback senza righe).
func loadedThread(refs []msg.MessageRef, bodies map[msg.MessageID]*msg.Message, m *msg.Message) ([]msg.MessageRef, map[msg.MessageID]*msg.Message) {
	if len(refs) == 0 && m != nil {
		return []msg.MessageRef{m.Ref}, map[msg.MessageID]*msg.Message{m.Ref.ID: m}
	}
	return refs, bodies
}

// fetchBody scarica il corpo del messaggio id richiesto dalla vista thread di riga
// anchor (Piano 8, Task 13). gen è la generazione del thread al momento della richiesta.
func fetchBody(st mail.MailStore, gen int, anchor, id msg.MessageID) tea.Cmd {
	return func() tea.Msg {
		mm, err := st.FetchMessage(context.Background(), id)
		return bodyLoaded{gen: gen, anchor: anchor, id: id, m: mm, err: err}
	}
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
