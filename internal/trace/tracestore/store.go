// Package tracestore avvolge un mail.MailStore (e un Sender) misurando ogni
// chiamata per la traccia dei tempi (Piano 16): durata totale e, per lo store
// IMAP, l'attesa del gate (Span nel ctx, cfr. imap.Store.acquire), con conteggi
// e byte. PRIVACY: nessun nome di label o cartella, id, indirizzo, oggetto o
// testo di errore: solo il ruolo (inbox/sent/drafts/all/trash/junk/flagged/label).
package tracestore

import (
	"context"
	"errors"
	"sync"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/trace"
)

// Sender e il contratto di invio (stesso di app.Sender: il pacchetto app non si
// importa da qui).
type Sender interface {
	Send(ctx context.Context, m msg.Outgoing) error
}

// Wrap avvolge st; senza traccia installata ritorna st stesso (nessun costo).
func Wrap(st mail.MailStore) mail.MailStore {
	if !trace.Enabled() {
		return st
	}
	return &store{in: st, roles: map[string]string{}}
}

// WrapSender avvolge sd; senza traccia ritorna sd stesso.
func WrapSender(sd Sender) Sender {
	if !trace.Enabled() {
		return sd
	}
	return sender{sd}
}

type sender struct{ in Sender }

func (s sender) Send(ctx context.Context, m msg.Outgoing) error {
	t := trace.Begin()
	err := s.in.Send(ctx, m)
	t.End(trace.Event{K: "store", Op: "Sender.Send", N: len(m.To) + len(m.Cc) + len(m.Bcc), Bytes: len(m.BodyText), Err: err != nil})
	return err
}

type store struct {
	in mail.MailStore

	mu    sync.Mutex
	roles map[string]string // nome cartella -> ruolo; resta qui, non si scrive mai
}

// GmailExt inoltra l'interfaccia opzionale gmailAware dell'app: senza
// quella dello store interno vale false, come se l'interfaccia mancasse.
func (s *store) GmailExt() bool {
	g, ok := s.in.(interface{ GmailExt() bool })
	return ok && g.GmailExt()
}

// Piano 16, Task 2: il wrapper inoltra mail.Snapshotter allo store interno
// (stesso schema di GmailExt); se l'interno non lo implementa risponde miss.
// Piano 16, Task 7: ogni lettura emette un evento cache (esito e ruolo, mai ID).
var _ mail.Snapshotter = (*store)(nil)

// cacheMode traduce l'esito di una lettura dalla cache nella modalita della traccia.
func cacheMode(ok bool) string {
	if ok {
		return trace.ModeHit
	}
	return trace.ModeMiss
}

func (s *store) CachedConversations(mailbox string) (mail.ConversationsSnapshot, bool) {
	var r mail.ConversationsSnapshot
	ok := false
	if sn, is := s.in.(mail.Snapshotter); is {
		r, ok = sn.CachedConversations(mailbox)
	}
	n := 0
	if ok {
		n = len(r.Rows)
	}
	trace.CacheLookup(trace.OpSnapList, cacheMode(ok), s.role(mailbox), n, 0)
	return r, ok
}

func (s *store) CachedThread(threadID uint64) (mail.ThreadSnapshot, bool) {
	var r mail.ThreadSnapshot
	ok := false
	if sn, is := s.in.(mail.Snapshotter); is {
		r, ok = sn.CachedThread(threadID)
	}
	n := 0
	if ok {
		n = len(r.Refs)
	}
	trace.CacheLookup(trace.OpSnapThread, cacheMode(ok), "", n, 0)
	return r, ok
}

func (s *store) CachedMessage(id msg.MessageID) (*msg.Message, bool) {
	var m *msg.Message
	ok := false
	if sn, is := s.in.(mail.Snapshotter); is {
		m, ok = sn.CachedMessage(id)
	}
	n, b := 0, 0
	if ok && m != nil {
		n, b = 1, len(m.BodyText)+len(m.BodyHTML)
	}
	role := ""
	if mb, _, err := id.Split(); err == nil {
		role = s.role(mb)
	}
	trace.CacheLookup(trace.OpSnapBody, cacheMode(ok), role, n, b)
	return m, ok
}

// role traduce un nome di cartella nel ruolo da tracciare. Vuoto = INBOX; un nome
// non noto (ListMailboxes non ancora letto) vale "label".
func (s *store) role(name string) string {
	if name == "" {
		return "inbox"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if r := s.roles[name]; r != "" {
		return r
	}
	return "label"
}

// call e una chiamata in corso: ctx con lo Span del gate e il timer.
type call struct {
	t  trace.Timer
	sp *trace.Span
	ev trace.Event
}

func begin(ctx context.Context, op string) (context.Context, *call) {
	ctx, sp := trace.WithSpan(ctx)
	return ctx, &call{t: trace.Begin(), sp: sp, ev: trace.Event{K: "store", Op: op}}
}

// end chiude la chiamata: n conteggio, bytes byte del corpo, err l'esito (solo il
// booleano; ErrScanLimit e una lista valida ma limitata, non un errore).
func (c *call) end(n, bytes int, err error) {
	c.ev.N, c.ev.Bytes = n, bytes
	c.ev.Err = err != nil && !errors.Is(err, mail.ErrScanLimit)
	c.ev.Wait = c.sp.Wait().Microseconds()
	c.t.End(c.ev)
}

func (s *store) Connect(ctx context.Context) error {
	ctx, c := begin(ctx, "Connect")
	err := s.in.Connect(ctx)
	c.end(0, 0, err)
	return err
}

func (s *store) ListMessages(ctx context.Context, q msg.Query, pg mail.Page) ([]msg.MessageRef, error) {
	ctx, c := begin(ctx, "ListMessages")
	c.queryInfo(s, q)
	r, err := s.in.ListMessages(ctx, q, pg)
	c.end(len(r), 0, err)
	return r, err
}

// queryInfo annota il ruolo della cartella e, per le query Gmail grezze, Mode "search".
func (c *call) queryInfo(s *store, q msg.Query) {
	c.ev.Role = s.role(q.Label)
	if q.Raw != "" {
		c.ev.Mode = "search"
	}
}

func (s *store) FetchMessage(ctx context.Context, id msg.MessageID) (*msg.Message, error) {
	ctx, c := begin(ctx, "FetchMessage")
	m, err := s.in.FetchMessage(ctx, id)
	b := 0
	if m != nil {
		b = len(m.BodyText) + len(m.BodyHTML)
	}
	c.end(1, b, err)
	return m, err
}

func (s *store) Search(ctx context.Context, q msg.Query) ([]msg.MessageRef, error) {
	ctx, c := begin(ctx, "Search")
	c.queryInfo(s, q)
	r, err := s.in.Search(ctx, q)
	c.end(len(r), 0, err)
	return r, err
}

func (s *store) ListConversations(ctx context.Context, q msg.Query, pg mail.Page) ([]mail.Conversation, error) {
	ctx, c := begin(ctx, "ListConversations")
	c.queryInfo(s, q)
	r, err := s.in.ListConversations(ctx, q, pg)
	c.end(len(r), 0, err)
	return r, err
}

func (s *store) ConversationMembers(ctx context.Context, mailbox string, threadID uint64) ([]msg.MessageID, error) {
	ctx, c := begin(ctx, "ConversationMembers")
	c.ev.Role = s.role(mailbox)
	r, err := s.in.ConversationMembers(ctx, mailbox, threadID)
	c.end(len(r), 0, err)
	return r, err
}

func (s *store) ThreadMessages(ctx context.Context, threadID uint64) ([]msg.MessageRef, error) {
	ctx, c := begin(ctx, "ThreadMessages")
	r, err := s.in.ThreadMessages(ctx, threadID)
	c.end(len(r), 0, err)
	return r, err
}

func (s *store) MarkRead(ctx context.Context, ids []msg.MessageID, read bool) error {
	ctx, c := begin(ctx, "MarkRead")
	err := s.in.MarkRead(ctx, ids, read)
	c.end(len(ids), 0, err)
	return err
}

func (s *store) Archive(ctx context.Context, ids []msg.MessageID) error {
	ctx, c := begin(ctx, "Archive")
	err := s.in.Archive(ctx, ids)
	c.end(len(ids), 0, err)
	return err
}

func (s *store) Star(ctx context.Context, ids []msg.MessageID, on bool) error {
	ctx, c := begin(ctx, "Star")
	err := s.in.Star(ctx, ids, on)
	c.end(len(ids), 0, err)
	return err
}

func (s *store) ApplyLabel(ctx context.Context, ids []msg.MessageID, label string, on bool) error {
	ctx, c := begin(ctx, "ApplyLabel")
	err := s.in.ApplyLabel(ctx, ids, label, on)
	c.end(len(ids), 0, err)
	return err
}

func (s *store) SaveAttachment(ctx context.Context, id msg.MessageID, part int, path string) error {
	ctx, c := begin(ctx, "SaveAttachment")
	err := s.in.SaveAttachment(ctx, id, part, path)
	c.end(1, 0, err)
	return err
}

func (s *store) Send(ctx context.Context, m msg.Outgoing) error {
	ctx, c := begin(ctx, "Send")
	err := s.in.Send(ctx, m)
	c.end(len(m.To)+len(m.Cc)+len(m.Bcc), len(m.BodyText), err)
	return err
}

// ListMailboxes impara anche la mappa nome -> ruolo usata da role: la mappa non
// esce mai dal wrapper.
func (s *store) ListMailboxes(ctx context.Context) ([]mail.Mailbox, error) {
	ctx, c := begin(ctx, "ListMailboxes")
	r, err := s.in.ListMailboxes(ctx)
	if err == nil {
		s.mu.Lock()
		for _, b := range r {
			s.roles[b.Name] = b.Role
		}
		s.mu.Unlock()
	}
	c.end(len(r), 0, err)
	return r, err
}

func (s *store) UnreadCounts(ctx context.Context) (map[string]int, error) {
	ctx, c := begin(ctx, "UnreadCounts")
	r, err := s.in.UnreadCounts(ctx)
	c.end(len(r), 0, err)
	return r, err
}

func (s *store) Trash(ctx context.Context, ids []msg.MessageID) ([]msg.MessageID, error) {
	ctx, c := begin(ctx, "Trash")
	r, err := s.in.Trash(ctx, ids)
	c.end(len(ids), 0, err)
	return r, err
}

func (s *store) Restore(ctx context.Context, ids []msg.MessageID, dest string) ([]msg.MessageID, error) {
	ctx, c := begin(ctx, "Restore")
	c.ev.Role = s.role(dest)
	r, err := s.in.Restore(ctx, ids, dest)
	c.end(len(ids), 0, err)
	return r, err
}

func (s *store) AllMailIDs(ctx context.Context, ids []msg.MessageID) ([]msg.MessageID, error) {
	ctx, c := begin(ctx, "AllMailIDs")
	r, err := s.in.AllMailIDs(ctx, ids)
	c.end(len(ids), 0, err)
	return r, err
}

func (s *store) ModifyLabels(ctx context.Context, ids []msg.MessageID, add, remove []string) error {
	ctx, c := begin(ctx, "ModifyLabels")
	err := s.in.ModifyLabels(ctx, ids, add, remove)
	c.end(len(ids), 0, err)
	return err
}

func (s *store) SaveDraft(ctx context.Context, raw []byte, replace []msg.MessageID) (msg.MessageID, error) {
	ctx, c := begin(ctx, "SaveDraft")
	id, err := s.in.SaveDraft(ctx, raw, replace)
	c.end(1, len(raw), err)
	return id, err
}

func (s *store) DeleteDraft(ctx context.Context, ids []msg.MessageID) error {
	ctx, c := begin(ctx, "DeleteDraft")
	err := s.in.DeleteDraft(ctx, ids)
	c.end(len(ids), 0, err)
	return err
}

// Watch non e un'operazione a durata nota (il canale vive finche il ctx): si
// misura solo la chiamata, che include l'attesa della baseline sul gate.
func (s *store) Watch(ctx context.Context) (<-chan mail.Change, error) {
	ctx, c := begin(ctx, "Watch")
	ch, err := s.in.Watch(ctx)
	c.end(0, 0, err)
	return ch, err
}
