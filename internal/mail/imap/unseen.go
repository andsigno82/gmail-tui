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
// fermi a lungo.
const unseenBudget = 10 * time.Second

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
	return unreadCounts(ctx, c)
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
	data, err := listData(c, withStatus)
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

		n, err := unseenOf(c, d, selected, opts)
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
func unseenOf(c *imapclient.Client, d *imap.ListData, selected string, opts *imap.StatusOptions) (int, error) {
	if sameMailbox(d.Mailbox, selected) {
		data, err := c.Search(&imap.SearchCriteria{NotFlag: []imap.Flag{imap.FlagSeen}}, nil).Wait()
		if err != nil {
			return 0, fmt.Errorf("imap search unseen in %s: %w", d.Mailbox, err)
		}
		return len(data.AllSeqNums()), nil
	}
	if d.Status != nil && d.Status.NumUnseen != nil {
		return int(*d.Status.NumUnseen), nil
	}
	st, err := c.Status(d.Mailbox, opts).Wait()
	if err != nil {
		return 0, fmt.Errorf("imap status %s: %w", d.Mailbox, err)
	}
	if st.NumUnseen == nil {
		return 0, &imap.Error{Type: imap.StatusResponseTypeNo, Text: "STATUS senza UNSEEN"}
	}
	return int(*st.NumUnseen), nil
}

// sameMailbox confronta due nomi di mailbox: INBOX è case insensitive (IMAP),
// gli altri nomi sono esatti. selected vuoto = nessuna mailbox selezionata.
func sameMailbox(a, b string) bool {
	if b == "" {
		return false
	}
	return a == b || (strings.EqualFold(a, "INBOX") && strings.EqualFold(b, "INBOX"))
}
