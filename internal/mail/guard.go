package mail

import (
	"context"
	"errors"
	"sync"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Guard avvolge st con la guardia degli ID confermati (Piano 16, Task 5, regola
// R4, difesa in profondità). Un MessageID è confermato solo se è uscito da una
// risposta FRESCA del server in questa sessione: ListMessages, Search,
// ListConversations (Latest.ID), ThreadMessages, ConversationMembers, Trash,
// Restore, AllMailIDs, SaveDraft e gli eventi del Watch (IDs e Refs).
//
// Ogni metodo che riceve un MessageID (letto per UID o scritto) con anche un solo
// ID non confermato ritorna ErrNotConfirmed SENZA chiamare lo store interno,
// quindi senza rete. Gli ID letti dalle istantanee (Snapshotter) non sono mai
// registrati. Le letture senza ID, Connect, Send, ListMailboxes e UnreadCounts
// passano sempre. Un Connect riuscito apre una sessione nuova e azzera le
// conferme; un cambio di UIDVALIDITY a sessione in corso resta fuori (Piano 16,
// limite dichiarato sotto R4).
//
// Snapshotter e GmailExt dello store interno sono inoltrati (stesso schema di
// tracestore): senza l'interfaccia interna rispondono miss / false.
func Guard(st MailStore) MailStore {
	return &guard{in: st, confirmed: map[msg.MessageID]struct{}{}}
}

type guard struct {
	in MailStore

	mu        sync.RWMutex
	confirmed map[msg.MessageID]struct{}
}

var (
	_ MailStore   = (*guard)(nil)
	_ Snapshotter = (*guard)(nil)
)

// confirm registra gli ID usciti da una risposta fresca.
func (g *guard) confirm(ids ...msg.MessageID) {
	if len(ids) == 0 {
		return
	}
	g.mu.Lock()
	for _, id := range ids {
		g.confirmed[id] = struct{}{}
	}
	g.mu.Unlock()
}

func (g *guard) confirmRefs(refs []msg.MessageRef) {
	if len(refs) == 0 {
		return
	}
	g.mu.Lock()
	for _, r := range refs {
		g.confirmed[r.ID] = struct{}{}
	}
	g.mu.Unlock()
}

// check ritorna ErrNotConfirmed se anche uno solo degli ID non è confermato.
func (g *guard) check(ids ...msg.MessageID) error {
	g.mu.RLock()
	defer g.mu.RUnlock()
	for _, id := range ids {
		if _, ok := g.confirmed[id]; !ok {
			return ErrNotConfirmed
		}
	}
	return nil
}

// fresh dice se una lettura ha prodotto righe valide: senza errore, oppure con
// ErrScanLimit (righe valide ma lista limitata). Ogni altro errore non conferma.
func fresh(err error) bool { return err == nil || errors.Is(err, ErrScanLimit) }

// Connect: un Connect riuscito azzera le conferme (sessione nuova).
func (g *guard) Connect(ctx context.Context) error {
	err := g.in.Connect(ctx)
	if err == nil {
		g.mu.Lock()
		g.confirmed = map[msg.MessageID]struct{}{}
		g.mu.Unlock()
	}
	return err
}

func (g *guard) ListMessages(ctx context.Context, q msg.Query, pg Page) ([]msg.MessageRef, error) {
	refs, err := g.in.ListMessages(ctx, q, pg)
	if fresh(err) {
		g.confirmRefs(refs)
	}
	return refs, err
}

func (g *guard) FetchMessage(ctx context.Context, id msg.MessageID) (*msg.Message, error) {
	if err := g.check(id); err != nil {
		return nil, err
	}
	return g.in.FetchMessage(ctx, id)
}

func (g *guard) Search(ctx context.Context, q msg.Query) ([]msg.MessageRef, error) {
	refs, err := g.in.Search(ctx, q)
	if fresh(err) {
		g.confirmRefs(refs)
	}
	return refs, err
}

func (g *guard) ListConversations(ctx context.Context, q msg.Query, pg Page) ([]Conversation, error) {
	rows, err := g.in.ListConversations(ctx, q, pg)
	if fresh(err) {
		for _, c := range rows {
			g.confirm(c.Latest.ID)
		}
	}
	return rows, err
}

func (g *guard) ConversationMembers(ctx context.Context, mailbox string, threadID uint64) ([]msg.MessageID, error) {
	ids, err := g.in.ConversationMembers(ctx, mailbox, threadID)
	if fresh(err) {
		g.confirm(ids...)
	}
	return ids, err
}

func (g *guard) ThreadMessages(ctx context.Context, threadID uint64) ([]msg.MessageRef, error) {
	refs, err := g.in.ThreadMessages(ctx, threadID)
	if fresh(err) {
		g.confirmRefs(refs)
	}
	return refs, err
}

func (g *guard) MarkRead(ctx context.Context, ids []msg.MessageID, read bool) error {
	if err := g.check(ids...); err != nil {
		return err
	}
	return g.in.MarkRead(ctx, ids, read)
}

func (g *guard) Archive(ctx context.Context, ids []msg.MessageID) error {
	if err := g.check(ids...); err != nil {
		return err
	}
	return g.in.Archive(ctx, ids)
}

func (g *guard) Star(ctx context.Context, ids []msg.MessageID, on bool) error {
	if err := g.check(ids...); err != nil {
		return err
	}
	return g.in.Star(ctx, ids, on)
}

func (g *guard) ApplyLabel(ctx context.Context, ids []msg.MessageID, label string, on bool) error {
	if err := g.check(ids...); err != nil {
		return err
	}
	return g.in.ApplyLabel(ctx, ids, label, on)
}

func (g *guard) SaveAttachment(ctx context.Context, id msg.MessageID, part int, path string) error {
	if err := g.check(id); err != nil {
		return err
	}
	return g.in.SaveAttachment(ctx, id, part, path)
}

// Send non porta ID: passa.
func (g *guard) Send(ctx context.Context, m msg.Outgoing) error { return g.in.Send(ctx, m) }

// ListMailboxes non porta ID: passa.
func (g *guard) ListMailboxes(ctx context.Context) ([]Mailbox, error) {
	return g.in.ListMailboxes(ctx)
}

// UnreadCounts non porta ID: passa.
func (g *guard) UnreadCounts(ctx context.Context) (map[string]int, error) {
	return g.in.UnreadCounts(ctx)
}

// Trash: gli ID in ingresso devono essere confermati; quelli nel cestino in
// uscita vengono dal server e si registrano (servono all'annulla).
func (g *guard) Trash(ctx context.Context, ids []msg.MessageID) ([]msg.MessageID, error) {
	if err := g.check(ids...); err != nil {
		return nil, err
	}
	out, err := g.in.Trash(ctx, ids)
	if err == nil {
		g.confirm(out...)
	}
	return out, err
}

func (g *guard) Restore(ctx context.Context, ids []msg.MessageID, dest string) ([]msg.MessageID, error) {
	if err := g.check(ids...); err != nil {
		return nil, err
	}
	out, err := g.in.Restore(ctx, ids, dest)
	if err == nil {
		g.confirm(out...)
	}
	return out, err
}

func (g *guard) AllMailIDs(ctx context.Context, ids []msg.MessageID) ([]msg.MessageID, error) {
	if err := g.check(ids...); err != nil {
		return nil, err
	}
	out, err := g.in.AllMailIDs(ctx, ids)
	if err == nil {
		g.confirm(out...)
	}
	return out, err
}

func (g *guard) ModifyLabels(ctx context.Context, ids []msg.MessageID, add, remove []string) error {
	if err := g.check(ids...); err != nil {
		return err
	}
	return g.in.ModifyLabels(ctx, ids, add, remove)
}

// SaveDraft: gli ID in replace devono essere confermati; l'ID della nuova bozza
// viene dal server e si registra.
func (g *guard) SaveDraft(ctx context.Context, raw []byte, replace []msg.MessageID) (msg.MessageID, error) {
	if err := g.check(replace...); err != nil {
		return "", err
	}
	id, err := g.in.SaveDraft(ctx, raw, replace)
	if err == nil && id != "" {
		g.confirm(id)
	}
	return id, err
}

func (g *guard) DeleteDraft(ctx context.Context, ids []msg.MessageID) error {
	if err := g.check(ids...); err != nil {
		return err
	}
	return g.in.DeleteDraft(ctx, ids)
}

// Watch rilancia il canale dello store interno registrando IDs e Refs di ogni
// evento PRIMA di consegnarlo. Contratto del canale rispettato: uno per chiamata,
// chiuso quando quello interno si chiude (cioè a ctx.Done); bufferizzato come
// l'interno. Dopo ctx.Done il rilancio continua a svuotare l'interno (e a
// registrare) senza più consegnare, così il produttore non resta bloccato.
func (g *guard) Watch(ctx context.Context) (<-chan Change, error) {
	in, err := g.in.Watch(ctx)
	if err != nil {
		return nil, err
	}
	out := make(chan Change, cap(in))
	go func() {
		defer close(out)
		for c := range in {
			g.confirm(c.IDs...)
			g.confirmRefs(c.Refs)
			select {
			case out <- c:
			case <-ctx.Done():
			}
		}
	}()
	return out, nil
}

// GmailExt inoltra la capability dello store interno (come tracestore): senza,
// vale false.
func (g *guard) GmailExt() bool {
	x, ok := g.in.(interface{ GmailExt() bool })
	return ok && x.GmailExt()
}

// Snapshotter inoltrato senza registrare nulla (R4: gli ID delle istantanee non
// sono mai confermati); senza Snapshotter interno risponde miss.
func (g *guard) CachedConversations(mailbox string) (ConversationsSnapshot, bool) {
	if sn, ok := g.in.(Snapshotter); ok {
		return sn.CachedConversations(mailbox)
	}
	return ConversationsSnapshot{}, false
}

func (g *guard) CachedThread(threadID uint64) (ThreadSnapshot, bool) {
	if sn, ok := g.in.(Snapshotter); ok {
		return sn.CachedThread(threadID)
	}
	return ThreadSnapshot{}, false
}

func (g *guard) CachedMessage(id msg.MessageID) (*msg.Message, bool) {
	if sn, ok := g.in.(Snapshotter); ok {
		return sn.CachedMessage(id)
	}
	return nil, false
}
