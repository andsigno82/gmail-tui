package app

// Vista conversazione nel thread (Piano 8, Task 10): caricamento dei messaggi
// della conversazione e segna-come-letto dei suoi non letti in una sola op.

import (
	"context"
	"errors"
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
	return loadConv(ctx, st, ref, folder, false)
}

// errConvGone: la conversazione di una riga provvisoria non esiste piu sul server.
var errConvGone = errors.New("conversazione non più disponibile")

// loadConv e loadConversation; con prov (riga di una lista provvisoria, Piano 16,
// Task 10, D16-T13) lavora SOLO col ThreadID: X-GM-THRID e globale in Gmail,
// indipendente dall'epoca, mentre ref.ID (mailbox + UID d'epoca vecchia) non si
// passa mai al server (un FetchMessage(ref.ID) mostrerebbe un altro messaggio). Gli
// ID usati sono solo quelli che ThreadMessages e ConversationMembers restituiscono
// (confermati). Il chiamante non lo invoca con prov e ThreadID 0.
func loadConv(ctx context.Context, st mail.MailStore, ref msg.MessageRef, folder mail.Mailbox, prov bool) ([]msg.MessageRef, map[msg.MessageID]*msg.Message, error) {
	if ref.ThreadID == 0 {
		if prov {
			return nil, nil, errConvGone // difesa: senza ThreadID si legge solo la cache
		}
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
	if !prov {
		extra = append(extra, ref.ID)
	}
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
	if len(all) == 0 {
		return nil, nil, errConvGone
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

// cachedConversation costruisce la conversazione di ref dalla sola cache locale
// (Piano 16, Task 10, D16-T13): i ref di CachedThread (dedup per GmailID come
// copyKey, ordine cronologico) con i corpi di CachedMessage; i corpi mancanti
// restano vuoti ("caricamento…"). Senza ThreadID, il solo messaggio della riga, solo
// se la riga e provvisoria (per una riga fresca FetchMessage e gia cache-first).
// ok false = niente in cache. Il risultato e PROVVISORIO.
func cachedConversation(sn mail.Snapshotter, ref msg.MessageRef, prov bool) ([]msg.MessageRef, map[msg.MessageID]*msg.Message, bool) {
	if ref.ThreadID == 0 {
		if !prov {
			return nil, nil, false
		}
		mm, ok := sn.CachedMessage(ref.ID)
		if !ok || mm == nil {
			return nil, nil, false
		}
		return []msg.MessageRef{mm.Ref}, map[msg.MessageID]*msg.Message{mm.Ref.ID: mm}, true
	}
	snap, ok := sn.CachedThread(ref.ThreadID)
	if !ok || len(snap.Refs) == 0 {
		return nil, nil, false
	}
	have := make(map[string]bool, len(snap.Refs))
	all := make([]msg.MessageRef, 0, len(snap.Refs))
	for _, r := range snap.Refs {
		if !have[copyKey(r)] {
			have[copyKey(r)] = true
			all = append(all, r)
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Date.Before(all[j].Date) })
	bodies := make(map[msg.MessageID]*msg.Message, len(all))
	for _, r := range all {
		if mm, ok := sn.CachedMessage(r.ID); ok && mm != nil {
			bodies[r.ID] = mm
		}
	}
	return all, bodies, true
}

// fetchMsgCached e la Cmd dell'apertura da istantanea (nessuna Cmd senza
// Snapshotter o senza niente in cache: nessun messaggio). Solo disco, nessuna rete.
func fetchMsgCached(st mail.MailStore, ref msg.MessageRef, prov bool) tea.Cmd {
	sn, ok := st.(mail.Snapshotter)
	if !ok {
		return nil
	}
	return func() tea.Msg {
		refs, bodies, ok := cachedConversation(sn, ref, prov)
		if !ok {
			return nil
		}
		return msgLoaded{refs: refs, bodies: bodies, requested: ref.ID, cached: true}
	}
}

// fetchMsgFor e fetchMsg per una riga che puo essere provvisoria: con prov il
// caricamento fresco usa solo il ThreadID; senza ThreadID nessuna Cmd (solo cache).
func fetchMsgFor(st mail.MailStore, ref msg.MessageRef, folder mail.Mailbox, prov bool) tea.Cmd {
	if !prov {
		return fetchMsg(st, ref, folder)
	}
	if ref.ThreadID == 0 {
		return nil
	}
	return func() tea.Msg {
		refs, bodies, err := loadConv(context.Background(), st, ref, folder, true)
		return msgLoaded{refs: refs, bodies: bodies, err: err, requested: ref.ID}
	}
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
