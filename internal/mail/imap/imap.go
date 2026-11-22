package imap

import (
	"context"
	"fmt"
	"sync"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/mail/cache"
	"github.com/andsigno82/gmail-tui/internal/msg"
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
type Store struct {
	opts Options

	// mu protegge client e wakes (granularità GROSSOLANA e deliberata: la
	// SERIALIZZAZIONE vera tra operazioni e IDLE è del gate, quella tra i
	// comandi IMAP dell'encoder del client; il mutex serve solo a rendere
	// atomici lo swap del client nelle riconnessioni di Watch e
	// l'installazione del canale dei wake).
	mu     sync.Mutex
	client *imapclient.Client
	wakes  chan watchWake // canale del Watch attivo, nil se nessuno

	// gmail: il server annuncia X-GM-EXT-1 (Piano 8, Task 6). Scritto in Connect
	// e Close sotto mu; letto con gmailOn().
	gmail bool

	// headersFn (Piano 11, review punto 8) è il seam di test per la lettura delle
	// intestazioni dei nuovi messaggi: nil in produzione (si usa fetchRefs).
	headersFn func(mailbox string, uids []imap.UID) ([]msg.MessageRef, error)

	// gate arbitra l'uso della connessione: go-imap beta.9 non permette di
	// inviare comandi mentre l'IDLE gira (il server chiude la connessione
	// alla violazione) e SEARCH/STORE/EXPUNGE agiscono sulla mailbox
	// correntemente selezionata (due op che interleaveano i SELECT leggerebbero
	// o scriverebbero nella mailbox sbagliata): le ops acquisiscono il gate —
	// che rompe l'IDLE del Watch ED esclude le altre op — prima di toccare la
	// rete (cfr. idleGate).
	gate idleGate
}

// Store implementa MailStore a compile time (assert di conformenza:
// Search/Watch/Send compresi).
var _ mail.MailStore = (*Store)(nil)

// idleGate arbitra l'uso della connessione tra le operazioni dello Store
// (comandi IMAP) e l'IDLE del Watch, E tra le operazioni stesse (Piano 3,
// Task 1): in go-imap v2.0.0-beta.9 non si possono inviare comandi mentre
// l'IDLE è in corso (il server chiude la connessione alla violazione) e
// SEARCH/STORE/EXPUNGE agiscono sulla mailbox CORRENTEMENTE selezionata —
// due op concorrenti che interleaveano i SELECT leggerebbero o scriverebbero
// nella mailbox sbagliata. Contratto: OPERAZIONI SERIALIZZATE — UNA OP ALLA
// VOLTA (l'attesa della TUI su ops in coda è accettabile: ops brevi). Le ops
// chiamano acquire (che, se il Watch è in IDLE, chiude breakCh per chiederne
// l'uscita e attende che l'idle sia terminato, quindi attende anche che
// l'holder precedente abbia rilasciato); il Watch chiama waitToIdle prima di
// ogni Idle e idleDone alla fine di ogni round (qualunque causa). I comandi
// watch-side FUORI dai round di idle (baseline in Watch, ri-verifica nella
// riconnessione) girano anch'essi dentro un round acquire→release (cfr.
// watch.go).
type idleGate struct {
	mu      sync.Mutex
	cond    *sync.Cond    // derivata da mu
	holders int           // ops che vogliono la connessione (attive + in attesa): waitToIdle attende 0
	active  bool          // UNA op detiene la connessione (mutua esclusione op-vs-op)
	idling  bool          // il watch è dentro Idle()
	breakCh chan struct{} // chiuso per chiedere al watch di uscire dall'idle
	closed  bool          // Close(): acquire diventa no-op, il watch esce
}

// acquire blocca l'accesso alla connessione per un'operazione e ritorna la
// funzione di release (da chiamare con defer). MUTUA ESCLUSIONE TOTALE
// (Piano 3, Task 1): l'op attende `holders == 0 && !idling`, cioè né un'altra
// op (active) né l'IDLE devono essere in corso — l'holder della connessione
// è UNO alla volta; le op serializzate sono accettabili per una TUI.
// NON è rientrante: l'unico path annidato (Search → post-filtro
// HasAttachment → fetch di corpo) usa fetchMessage(..., heldGate=true) per
// girare sotto l'holder già detenuto da Search (cfr. fetch.go).
//
// holders è incrementato PRIMA di attendere: finché l'op non rilascia,
// waitToIdle non può rientrare in IDLE — senza questo, tra la idleDone del
// round rotto e la ripresa dell'op il Watch potrebbe rimarcare idling,
// l'op risvegliato vedrebbe idling di nuovo true e chiuderebbe il NUOVO
// breakCh a vuoto, restando in attesa (livelock osservato in debug, fix
// Piano 2 Task 7). Se il Watch è in IDLE, acquire chiude breakCh (una volta:
// ricreato al prossimo waitToIdle) e attende su cond; l'attesa è limitata dal
// tempo di uscita dall'IDLE (DONE su connessione sana: sub-second). Dopo
// Close acquire è un no-op (release nil-safe): il client è comunque nil e
// l'op fallirà con l'errore "non connesso"; la scelta evita di bloccare i
// chiamanti in fase di shutdown.
func (g *idleGate) acquire() func() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return func() {} // no-op, cfr. doc
	}
	g.holders++ // prima dell'attesa: il Watch non rienta in IDLE (vedi sopra)
	for g.active || g.idling {
		if g.idling && g.breakCh != nil {
			close(g.breakCh) // chiedi al watch di uscire dall'IDLE
			g.breakCh = nil
		}
		g.cond.Wait()
		if g.closed {
			g.holders--
			g.cond.Broadcast()
			return func() {} // chiuso mentre attendevamo: no-op
		}
	}
	g.active = true // l'holder è UNO: le altre op restano in attesa qui
	return func() {
		g.mu.Lock()
		g.active = false
		g.holders--
		g.mu.Unlock()
		g.cond.Broadcast()
	}
}

// waitToIdle è il lato Watch: attende (cond) finché ci sono ops che vogliono
// la connessione (attive o in attesa: holders == 0), poi marca idling e
// ritorna il breakCh del round (una nuova istanza a ogni round: la precedente
// può essere stata chiusa da acquire). ok=false ⇒ Store chiuso: il Watch deve
// terminare.
func (g *idleGate) waitToIdle() (breakCh chan struct{}, ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for g.holders > 0 && !g.closed {
		g.cond.Wait()
	}
	if g.closed {
		return nil, false
	}
	g.idling = true
	g.breakCh = make(chan struct{})
	return g.breakCh, true
}

// idleDone chiude il round di idle (qualunque sia la causa dell'uscita):
// libera il gate e risveglia le ops in attesa e l'eventuale waitToIdle.
func (g *idleGate) idleDone() {
	g.mu.Lock()
	g.idling = false
	g.breakCh = nil // ricreato al prossimo waitToIdle
	g.mu.Unlock()
	g.cond.Broadcast()
}

// close marca il gate come chiuso: rompe l'IDLE in corso (senza attendere
// gli holders) e fa diventare acquire no-op. È risvegliato anche chi era in
// attesa di idle.
func (g *idleGate) close() {
	g.mu.Lock()
	g.closed = true
	if g.breakCh != nil {
		close(g.breakCh)
		g.breakCh = nil
	}
	g.mu.Unlock()
	g.cond.Broadcast()
}

// reset riporta il gate a nuovo dopo un Close (per il riuso dello Store via
// Connect): closed=false, niente idling. holders e active non si toccano: le
// release in volo sono appaiate (chiuso-escape incluso) e si auto-correggono.
func (g *idleGate) reset() {
	g.mu.Lock()
	g.closed = false
	g.idling = false
	g.breakCh = nil
	g.mu.Unlock()
	g.cond.Broadcast()
}

// isIdling riferisce se il Watch è dentro Idle() (aiuto ai test: il test
// deve aspettare l'idle effettivo prima di piantare un'op concorrente).
func (g *idleGate) isIdling() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.idling
}

// NewStore crea uno Store non connesso: chiamare Connect.
func NewStore(o Options) *Store {
	s := &Store{opts: o}
	s.gate.cond = sync.NewCond(&s.gate.mu)
	return s
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
	s.gate.reset()                       // riese dopo Close: il gate torna operativo
	s.gmail = c.Caps().Has(capGmailExt1) // Piano 8, Task 6
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

// Close chiude la connessione (LOGOUT). Idempotente. Rompe l'eventuale IDLE
// del Watch senza attendere gli holders: dopo Close le ops non attendono più
// il gate (acquire no-op) e falliscono comunque con "non connesso". Il Watch
// va fermato chiudendo il suo ctx PRIMA di chiamare Close.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gate.close()
	s.gmail = false // Piano 8, Task 6
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
		// Piano 14, Wave 4: sentinel riconoscibile con errors.Is (mail.ErrNotConnected).
		return nil, fmt.Errorf("%w: chiamare Connect prima di questa operazione", mail.ErrNotConnected)
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
//
// Gate (fix review): l'intera sequenza va tenuta insieme — acquire rompe
// l'eventuale IDLE del Watch e la release (defer, fine metodo) libera il
// Watch solo a sequenza conclusa, così la SELECT non può essere scavalcata
// da una SELECT del Watch tra SEARCH e FETCH.
func (s *Store) ListMessages(ctx context.Context, q msg.Query, pg mail.Page) ([]msg.MessageRef, error) {
	if q.Raw != "" { // Piano 8, Task 8: ricerca X-GM-RAW paginata in memoria
		refs, err := s.searchRaw(ctx, q)
		if err != nil {
			return nil, err
		}
		return pageRefs(refs, pg), nil
	}
	defer s.gate.acquire()()

	mailbox, uids, err := s.selectAndSearch(ctx, q)
	if err != nil {
		return nil, err
	}

	if q.HasAttachment {
		// Piano 14, Wave 4 review (punto 2): post-filtro best-effort come Search (contratto
		// mail.MailStore), prima della paginazione. Il gate è già detenuto:
		// filterWithAttachment usa fetchMessage(heldGate=true).
		refs, err := s.fetchRefs(mailbox, uids)
		if err != nil {
			return nil, err
		}
		refs, err = s.filterWithAttachment(ctx, refs)
		if err != nil {
			return nil, err
		}
		limit := pg.Limit
		if limit == 0 {
			limit = defaultPageLimit
		}
		start, end := pageWindow(len(refs), pg.Offset, limit)
		if start >= end {
			return nil, nil
		}
		return refs[start:end], nil
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

	gm := s.gmailOn() // Piano 8, Task 8: X-GM-* solo se il server li annuncia
	cmd := c.Fetch(imap.UIDSetNum(uids...), &imap.FetchOptions{
		UID:           true,
		Envelope:      true,
		Flags:         true,
		InternalDate:  true,
		GmailMsgID:    gm,
		GmailThreadID: gm,
		GmailLabels:   gm,
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
		ref := toRef(mailbox, uids[i], byUID[uids[i]])
		if gm {
			setGmailFields(&ref, byUID[uids[i]])
		}
		refs = append(refs, ref)
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
