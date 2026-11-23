package imap

// Piano 16, Task 8: istantanee locali dello store IMAP. Lettura (mail.Snapshotter): sola
// lettura dal disco, senza gate, senza rete e senza s.mu, valida anche prima di Connect;
// risultati PROVVISORI (R1). Scrittura (write-through, D16-T2): la scrive lo store, vicino
// alla UIDVALIDITY della SELECT; l'app non scrive mai sul disco. Un errore della cache
// delle istantanee non e mai un errore dell'op: si ignora, come una cache assente.

import (
	"strings"
	"time"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/trace"
)

var _ mail.Snapshotter = (*Store)(nil)

// CachedConversations restituisce l'istantanea della prima pagina di mailbox ("" = INBOX).
// Cache assente, voce assente, illeggibile o d'epoca vecchia (R3) = miss.
func (s *Store) CachedConversations(mailbox string) (mail.ConversationsSnapshot, bool) {
	if s.opts.Cache == nil {
		return mail.ConversationsSnapshot{}, false
	}
	rows, at, limited, ok := s.opts.Cache.GetList(mailbox)
	if !ok {
		return mail.ConversationsSnapshot{}, false
	}
	return mail.ConversationsSnapshot{Rows: rows, SavedAt: at, Limited: limited}, true
}

// CachedThread restituisce l'istantanea dei ref del thread (X-GM-THRID).
func (s *Store) CachedThread(threadID uint64) (mail.ThreadSnapshot, bool) {
	if s.opts.Cache == nil {
		return mail.ThreadSnapshot{}, false
	}
	refs, at, ok := s.opts.Cache.GetThread(threadID)
	if !ok {
		return mail.ThreadSnapshot{}, false
	}
	return mail.ThreadSnapshot{Refs: refs, SavedAt: at}, true
}

// CachedMessage restituisce il corpo in cache di id, senza rete.
func (s *Store) CachedMessage(id msg.MessageID) (*msg.Message, bool) {
	if s.opts.Cache == nil {
		return nil, false
	}
	mailbox, uid, err := id.Split()
	if err != nil {
		return nil, false
	}
	m, ok, err := s.opts.Cache.GetMessage(mailbox, uid)
	if err != nil || !ok {
		return nil, false
	}
	return m, true
}

// now e l'orologio dello store (iniettabile nei test: SavedAt delle istantanee).
func (s *Store) now() time.Time {
	if s.clock != nil {
		return s.clock()
	}
	return time.Now()
}

// noteEpoch registra la UIDVALIDITY che il server ha appena dichiarato per mailbox
// (R3): se e diversa da quella in cache scarta corpi e istantanee della mailbox
// PRIMA che lo store restituisca righe. Va chiamata subito dopo ogni SELECT di lista,
// thread e fetch. UIDVALIDITY 0 = non dichiarata (server atipico): niente da registrare.
func (s *Store) noteEpoch(mailbox string, uidValidity uint32) {
	if s.opts.Cache == nil || uidValidity == 0 {
		return
	}
	changed, err := s.opts.Cache.NoteUIDValidity(mailbox, uidValidity)
	if err == nil && changed {
		trace.CacheLookup(trace.OpFetchBody, trace.ModeEpoch, "", 0, 0)
	}
}

// snapshotableList dice se una ListConversations va salvata (D16-T2): prima pagina e
// query di sola cartella (nessun altro criterio, Raw vuoto).
func snapshotableList(q msg.Query, pg mail.Page) bool {
	return pg.Offset == 0 && q == (msg.Query{Label: q.Label})
}

// saveList scrive l'istantanea della cartella. UIDVALIDITY 0: l'epoca non e verificabile,
// quindi non si salva (R3).
func (s *Store) saveList(mailbox string, uidValidity uint32, rows []mail.Conversation, limited bool) {
	if s.opts.Cache == nil || uidValidity == 0 {
		return
	}
	_ = s.opts.Cache.PutList(mailbox, uidValidity, rows, limited, s.now())
}

// saveThread scrive l'istantanea del thread (mailbox = Tutti i messaggi).
func (s *Store) saveThread(threadID uint64, mailbox string, uidValidity uint32, refs []msg.MessageRef) {
	if s.opts.Cache == nil || uidValidity == 0 {
		return
	}
	_ = s.opts.Cache.PutThread(threadID, mailbox, uidValidity, refs, s.now())
}

// snapIdentity ricava GmailID e ThreadID degli ids dai dati locali, senza rete: dal corpo
// in cache, altrimenti dall'ultimo messaggio delle righe dell'istantanea della mailbox
// dell'id. Gli id non risolti non danno nulla (l'istantanea resta com'e, e provvisoria).
func (s *Store) snapIdentity(ids []msg.MessageID) (gmailIDs, threadIDs []uint64) {
	c := s.opts.Cache
	if c == nil {
		return nil, nil
	}
	lists := map[string][]mail.Conversation{}
	for _, id := range ids {
		box, uid, err := id.Split()
		if err != nil {
			continue
		}
		if m, ok, err := c.GetMessage(box, uid); err == nil && ok {
			gmailIDs = append(gmailIDs, m.Ref.GmailID)
			threadIDs = append(threadIDs, m.Ref.ThreadID)
			continue
		}
		rows, loaded := lists[box]
		if !loaded {
			rows, _, _, _ = c.GetList(box)
			lists[box] = rows
		}
		for _, r := range rows {
			if r.Latest.ID == id {
				gmailIDs = append(gmailIDs, r.Latest.GmailID)
				threadIDs = append(threadIDs, r.ThreadID)
			}
		}
	}
	return gmailIDs, threadIDs
}

// snapRemove toglie ids dall'istantanea di lista di mailbox (righe per ThreadID o per
// ultimo messaggio) e, con dropThreadRefs (Trash), i messaggi dalle istantanee di thread
// (D16-T3). Chiamata dopo l'op riuscita.
func (s *Store) snapRemove(mailbox string, ids []msg.MessageID, dropThreadRefs bool) {
	c := s.opts.Cache
	if c == nil || len(ids) == 0 {
		return
	}
	gmailIDs, threadIDs := s.snapIdentity(ids)
	_ = c.RemoveListRows(mailbox, threadIDs, ids)
	if dropThreadRefs {
		_ = c.RemoveThreadRefs(gmailIDs)
	}
}

// snapPatch applica mutate ai ref degli ids nelle istantanee (righe e thread): stesso
// mutate per GmailID usato per i corpi (D16-T3).
func (s *Store) snapPatch(ids []msg.MessageID, mutate func(*msg.MessageRef) bool) {
	c := s.opts.Cache
	if c == nil || len(ids) == 0 {
		return
	}
	gmailIDs, _ := s.snapIdentity(ids)
	_ = c.PatchSnapshotRefs(gmailIDs, mutate)
}

// snapLabelSet aggiorna le istantanee dopo un ModifyLabels riuscito sugli ids: le label
// utente entrano o escono dai ref e, rimossa una cartella (\Inbox = INBOX) o una label,
// la conversazione esce dalla lista di quella mailbox (D16-T3).
func (s *Store) snapLabelSet(ids []msg.MessageID, add, remove []string) {
	if s.opts.Cache == nil || len(ids) == 0 {
		return
	}
	s.snapPatch(ids, labelSetMutate(add, remove))
	for _, r := range remove {
		box := r
		if r == `\Inbox` {
			box = "INBOX"
		}
		if strings.HasPrefix(box, `\`) {
			continue
		}
		s.snapRemove(box, ids, false)
	}
}

// labelSetMutate e il mutate dei ref per ModifyLabels/ApplyLabel: le label utente entrano
// o escono (gli attributi di sistema come \Inbox non sono label utente).
func labelSetMutate(add, remove []string) func(*msg.MessageRef) bool {
	userAdd, userRemove := msg.UserLabels(add), msg.UserLabels(remove)
	return func(r *msg.MessageRef) bool {
		labels := r.Labels
		for _, l := range userRemove {
			labels = withLabel(labels, l, false)
		}
		for _, l := range userAdd {
			labels = withLabel(labels, l, true)
		}
		changed := len(labels) != len(r.Labels)
		for i := 0; !changed && i < len(labels); i++ {
			changed = labels[i] != r.Labels[i]
		}
		r.Labels = labels
		return changed
	}
}
