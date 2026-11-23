package imap

// Contatori dei non letti per cartella (Piano 15, Task 11): STATUS (UNSEEN)
// sotto gate su INBOX e label utente. Strategia di costo: se il server annuncia
// LIST-STATUS (RFC 5819) basta UN LIST con RETURN (STATUS (UNSEEN)); altrimenti
// uno STATUS per cartella. La mailbox selezionata non riceve STATUS (RFC 3501
// §6.3.10: SHOULD NOT) ma un SEARCH UNSEEN. Niente SELECT: la mailbox che il
// Watch si aspetta resta quella di prima.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// unseenBudget è il tempo massimo (a gate preso) per il giro dei conteggi: oltre
// si restituisce la mappa parziale (le mailbox assenti = non calcolate, cfr.
// mail.MailStore.UnreadCounts) invece di tenere il gate, e quindi il Watch,
// fermi a lungo. Vale anche DENTRO LIST/STATUS/SEARCH (awaitBudget: scaduto il
// budget la connessione si chiude e il Watch riconnette).
var unseenBudget = 10 * time.Second // var: i test lo accorciano

// UnreadCounts implementa mail.MailStore (Piano 15, Task 11): mappa
// Mailbox.Name → messaggi non letti per INBOX e label utente (le speciali
// [Gmail]/* e i \Noselect sono escluse). Errore su una singola mailbox: saltata
// (best-effort); errore di connessione o ctx annullato: restituito. Sotto gate.
func (s *Store) UnreadCounts(ctx context.Context) (map[string]int, error) {
	defer s.gate.acquire()()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c, err := s.connected()
	if err != nil {
		return nil, err
	}
	counts, err := unreadCounts(ctx, c)
	select {
	case <-c.Closed():
		// awaitBudget ha chiuso la connessione (budget scaduto dentro un comando):
		// ci si riconnette PRIMA di rilasciare il gate, cosi le op successive e il
		// Watch trovano subito un client sano. Se il dial/login fallisce, client nil:
		// le op danno mail.ErrNotConnected e il Watch riprova col suo backoff.
		// Connect non prende il gate (reset lo lascia attivo: idling e gia false).
		if cerr := s.Connect(ctx); cerr != nil {
			s.mu.Lock()
			s.client = nil
			s.mu.Unlock()
		}
	default:
	}
	return counts, err
}

// unreadCounts è il corpo di UnreadCounts SENZA gate (cfr. listMailboxes).
func unreadCounts(ctx context.Context, c *imapclient.Client) (map[string]int, error) {
	budget, cancel := context.WithTimeout(ctx, unseenBudget)
	defer cancel()

	opts := &imap.StatusOptions{NumUnseen: true}
	caps := c.Caps()
	var withStatus *imap.StatusOptions // LIST RETURN (STATUS) solo se annunciato
	if caps.Has(imap.CapListStatus) && caps.Has(imap.CapListExtended) {
		withStatus = opts
	}
	data, err := awaitBudget(ctx, budget, c, func() ([]*imap.ListData, error) { return listData(c, withStatus) })
	if errors.Is(err, errBudget) {
		return map[string]int{}, nil // il LIST non e' arrivato in tempo: nulla da contare
	}
	if err != nil {
		return nil, err
	}

	selected := ""
	if m := c.Mailbox(); m != nil {
		selected = m.Name
	}

	counts := make(map[string]int, len(data))
	for _, d := range data {
		if hasAttr(d.Attrs, imap.MailboxAttrNoSelect) || (roleOf(d) != "inbox" && !msg.IsUserLabel(d.Mailbox)) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err // annullato dal chiamante
		}
		if budget.Err() != nil {
			break // budget esaurito: mappa parziale
		}

		n, err := unseenOf(ctx, budget, c, d, selected, opts)
		if errors.Is(err, errBudget) {
			break // budget esaurito dentro un comando: mappa parziale
		}
		if err != nil {
			var ie *imap.Error
			if errors.As(err, &ie) {
				continue // NO/BAD su questa mailbox: best-effort, si salta
			}
			return nil, err // rete/protocollo: la connessione non è affidabile
		}
		counts[d.Mailbox] = n
	}
	return counts, nil
}

// unseenOf dà i non letti di una mailbox listata: SEARCH se è la selezionata,
// altrimenti il risultato del LIST-STATUS se presente, altrimenti uno STATUS.
func unseenOf(ctx, budget context.Context, c *imapclient.Client, d *imap.ListData, selected string, opts *imap.StatusOptions) (int, error) {
	if sameMailbox(d.Mailbox, selected) {
		data, err := awaitBudget(ctx, budget, c, func() (*imap.SearchData, error) {
			return c.Search(&imap.SearchCriteria{NotFlag: []imap.Flag{imap.FlagSeen}}, nil).Wait()
		})
		if err != nil {
			return 0, fmt.Errorf("imap search unseen in %s: %w", d.Mailbox, err)
		}
		return len(data.AllSeqNums()), nil
	}
	if d.Status != nil && d.Status.NumUnseen != nil {
		return int(*d.Status.NumUnseen), nil
	}
	st, err := awaitBudget(ctx, budget, c, func() (*imap.StatusData, error) { return c.Status(d.Mailbox, opts).Wait() })
	if err != nil {
		return 0, fmt.Errorf("imap status %s: %w", d.Mailbox, err)
	}
	if st.NumUnseen == nil {
		return 0, &imap.Error{Type: imap.StatusResponseTypeNo, Text: "STATUS senza UNSEEN"}
	}
	return int(*st.NumUnseen), nil
}

// errBudget: il budget di unreadCounts e' scaduto dentro un comando in corso.
var errBudget = errors.New("imap: budget dei conteggi esaurito")

// awaitBudget attende l'esito di un comando bloccante (go-imap beta.9 non ha un
// Wait con contesto). Se il budget scade prima della risposta la connessione
// viene chiusa: sblocca il comando pendente e non lascia comandi in volo sotto
// un gate rilasciato (il Watch riconnette come per ogni errore di rete). Ritorna
// errBudget; se a scadere e' invece il ctx del chiamante, il suo errore
// (revisione Piano 15, punto 5).
func awaitBudget[T any](ctx, budget context.Context, c *imapclient.Client, wait func() (T, error)) (T, error) {
	type result struct {
		v   T
		err error
	}
	ch := make(chan result, 1) // bufferizzato: il goroutine termina anche se nessuno legge
	go func() {
		v, err := wait()
		ch <- result{v, err}
	}()
	select {
	case r := <-ch:
		return r.v, r.err
	case <-budget.Done():
		_ = c.Close()
		var zero T
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		return zero, errBudget
	}
}

// sameMailbox confronta due nomi di mailbox: INBOX è case insensitive (IMAP),
// gli altri nomi sono esatti. selected vuoto = nessuna mailbox selezionata.
func sameMailbox(a, b string) bool {
	if b == "" {
		return false
	}
	return a == b || (strings.EqualFold(a, "INBOX") && strings.EqualFold(b, "INBOX"))
}
