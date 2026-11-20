package imap

import (
	"context"
	"fmt"
	"sync"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"gmail-tui/internal/mail"
	"gmail-tui/internal/mail/cache"
	"gmail-tui/internal/msg"
)

// defaultPageLimit è il limite usato quando Page.Limit == 0.
const defaultPageLimit = 50

// Options configura uno Store.
type Options struct {
	Addr     string // es. "imap.gmail.com:993"
	Username string
	Password string
	Cache    *cache.Cache // può essere nil (no cache)
	Insecure bool         // true solo nei test: connessione in chiaro, nessun TLS
}

// Store è il mail.MailStore su IMAP (Gmail). Semantica Gmail via IMAP:
// le label sono cartelle; COPY sulla cartella aggiunge la label;
// \Deleted+EXPUNGE nella cartella rimuove la label (= archivia da INBOX,
// il messaggio resta in All Mail).
//
// NOTA: l'assert di conformenza `var _ mail.MailStore = (*Store)(nil)`
// previsto dalla verifica finale del piano viene aggiunto nel Task 8:
// dopo Watch (Task 7) manca ancora Send all'interfaccia.
type Store struct {
	opts Options

	// mu protegge client e wakes (granularità GROSSOLANA e deliberata: le
	// operazioni IMAP vere e proprie restano serializzate dall'encoder del
	// client; il mutex serve solo a rendere atomici lo swap del client nelle
	// riconnessioni di Watch e l'installazione del canale dei wake).
	mu     sync.Mutex
	client *imapclient.Client
	wakes  chan watchWake // canale del Watch attivo, nil se nessuno
}

// NewStore crea uno Store non connesso: chiamare Connect.
func NewStore(o Options) *Store {
	return &Store{opts: o}
}

// Connect apre la connessione (TLS, o in chiaro se Options.Insecure) ed
// effettua il login. Se esiste già un client (doppio Connect, o il path di
// riconnessione di Watch dopo un errore di rete) viene chiuso PRIMA di
// diallare: così un secondo Connect non leaka la connessione precedente.
func (s *Store) Connect(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.client != nil {
		// Già connesso: chiudi la vecchia sessione (il suo errore non è
		// rilevante, la stiamo sostituendo).
		_ = s.client.Close()
		s.client = nil
	}

	dial := imapclient.DialTLS
	if s.opts.Insecure {
		dial = imapclient.DialInsecure
	}
	c, err := dial(s.opts.Addr, s.dialOptions())
	if err != nil {
		return fmt.Errorf("imap dial %s: %w", s.opts.Addr, err)
	}
	if err := c.Login(s.opts.Username, s.opts.Password).Wait(); err != nil {
		_ = c.Close()
		return fmt.Errorf("imap login %s: %w", s.opts.Username, err)
	}
	s.client = c
	return nil
}

// dialOptions costruisce le imapclient.Options per il dial: installa i
// gestori dei dati unilaterali (EXISTS/FETCH/EXPUNGE non richiesti da un
// comando pendente) che alimentano il Watch. Va chiamata con mu presa.
func (s *Store) dialOptions() *imapclient.Options {
	return &imapclient.Options{
		UnilateralDataHandler: s.unilateralHandler(),
	}
}

// Close chiude la connessione (LOGOUT). Idempotente.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client == nil {
		return nil
	}
	err := s.client.Close()
	s.client = nil
	if err != nil {
		return fmt.Errorf("imap close: %w", err)
	}
	return nil
}

// connected restituisce il client collegato o un errore descrittivo.
func (s *Store) connected() (*imapclient.Client, error) {
	s.mu.Lock()
	c := s.client
	s.mu.Unlock()
	if c == nil {
		return nil, fmt.Errorf("imap store non connesso: chiamare Connect prima di questa operazione")
	}
	return c, nil
}

// ListMessages restituisce una pagina di righe di lista secondo q e pg.
//
// Comportamento (piano 2, Task 3):
//  1. Select della mailbox q.Label (o "INBOX" se vuota);
//  2. UIDSearch con i criteri tradotti da q (query vuota = match-all);
//     paginazione dal più nuovo: la lista UID è ascendente, la pagina è la
//     finestra [len-offset-limit, len-offset) (offset 0 = più recenti),
//     Limit 0 ⇒ 50;
//  3. UIDFetch ENVELOPE+FLAGS+INTERNALDATE solo per gli UID della pagina;
//  4. mapping in msg.MessageRef (ThreadLen = 1: ENVELOPE non espone il
//     thread, il raggruppamento è responsabilità della UI);
//  5. ritorno dal più nuovo al più vecchio.
func (s *Store) ListMessages(ctx context.Context, q msg.Query, pg mail.Page) ([]msg.MessageRef, error) {
	mailbox, uids, err := s.selectAndSearch(ctx, q)
	if err != nil {
		return nil, err
	}

	limit := pg.Limit
	if limit == 0 {
		limit = defaultPageLimit
	}
	start, end := pageWindow(len(uids), pg.Offset, limit)
	if start >= end {
		return nil, nil
	}
	return s.fetchRefs(mailbox, uids[start:end])
}

// selectAndSearch è il passo 1-2 condiviso da ListMessages e Search:
// Select della mailbox di q (q.Label o "INBOX" se vuota: la label seleziona
// la mailbox, non è un criterio SEARCH) e UIDSearch con i criteri tradotti
// da searchCriteria(q). Ritorna gli UID matchanti in ordine ascendente.
func (s *Store) selectAndSearch(ctx context.Context, q msg.Query) (string, []imap.UID, error) {
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	c, err := s.connected()
	if err != nil {
		return "", nil, err
	}

	mailbox := "INBOX"
	if q.Label != "" {
		mailbox = q.Label
	}
	if _, err := c.Select(mailbox, nil).Wait(); err != nil {
		return "", nil, fmt.Errorf("imap select %s: %w", mailbox, err)
	}

	data, err := c.UIDSearch(searchCriteria(q), nil).Wait()
	if err != nil {
		return "", nil, fmt.Errorf("imap uid search in %s: %w", mailbox, err)
	}
	return mailbox, data.AllUIDs(), nil // UID ascendenti
}

// fetchRefs è il passo 3-5 condiviso da ListMessages e Search: UIDFetch
// ENVELOPE+FLAGS+INTERNALDATE degli uid (nella mailbox già selezionata) e
// mapping in msg.MessageRef, dal più nuovo al più vecchio.
func (s *Store) fetchRefs(mailbox string, uids []imap.UID) ([]msg.MessageRef, error) {
	if len(uids) == 0 {
		return nil, nil
	}
	c, err := s.connected()
	if err != nil {
		return nil, err
	}

	cmd := c.Fetch(imap.UIDSetNum(uids...), &imap.FetchOptions{
		UID:          true,
		Envelope:     true,
		Flags:        true,
		InternalDate: true,
	})
	bufs, err := cmd.Collect()
	if err != nil {
		return nil, fmt.Errorf("imap fetch envelope in %s: %w", mailbox, err)
	}
	byUID := make(map[imap.UID]*imapclient.FetchMessageBuffer, len(bufs))
	for _, b := range bufs {
		byUID[b.UID] = b
	}

	refs := make([]msg.MessageRef, 0, len(uids))
	for i := len(uids) - 1; i >= 0; i-- { // dal più nuovo al più vecchio
		refs = append(refs, toRef(mailbox, uids[i], byUID[uids[i]]))
	}
	return refs, nil
}

// pageWindow calcola la finestra [start,end) sul v0 di una lista ascendente
// di lunghezza n: offset 0 = elementi più recenti (in coda), limit elementi.
func pageWindow(n, offset, limit int) (start, end int) {
	end = n - offset
	if end < 0 {
		end = 0
	}
	start = end - limit
	if start < 0 {
		start = 0
	}
	return start, end
}

// toRef mappa un buffer di fetch in msg.MessageRef.
func toRef(mailbox string, uid imap.UID, buf *imapclient.FetchMessageBuffer) msg.MessageRef {
	ref := msg.MessageRef{
		ID:        msg.NewID(mailbox, uint32(uid)),
		Unread:    true, // Unread = assenza di \Seen
		ThreadLen: 1,    // ENVELOPE non espone la lunghezza del thread
	}
	if buf == nil {
		return ref
	}
	if buf.Envelope != nil {
		ref.Subject = buf.Envelope.Subject
		ref.From = formatAddress(buf.Envelope.From)
		ref.Date = buf.InternalDate
		if ref.Date.IsZero() {
			ref.Date = buf.Envelope.Date // fallback: data dell'envelope
		}
	}
	for _, f := range buf.Flags {
		switch f {
		case imap.FlagSeen:
			ref.Unread = false
		case imap.FlagFlagged:
			ref.Starred = true
		}
	}
	return ref
}

// formatAddress rende il primo indirizzo come "Name <addr>", o l'indirizzo
// nudo senza nome; stringa vuota se assente.
func formatAddress(addrs []imap.Address) string {
	if len(addrs) == 0 {
		return ""
	}
	a := &addrs[0]
	addr := a.Addr()
	switch {
	case addr == "":
		return "" // marker di gruppo o assente
	case a.Name == "":
		return addr
	default:
		return a.Name + " <" + addr + ">"
	}
}
