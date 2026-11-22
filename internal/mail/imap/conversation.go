package imap

// Piano 8, Task 7: conversazioni Gmail (X-GM-THRID). Una riga per thread, come
// Gmail: Count, Participants, Unread e Labels valgono su All Mail (spam e cestino
// esclusi da Gmail stesso). Senza X-GM-EXT-1 ogni messaggio è una riga.
//
// Paginazione (D8-3, variante documentata): Page.Offset e Page.Limit contano
// CONVERSAZIONI. Ogni chiamata scansiona i messaggi dal più recente fino a
// riempire offset+limit thread distinti, con tetto di maxMessaggiScansionati.
// Costo O(offset) per pagina; in cambio nessuno stato tra le chiamate.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

const (
	conversazioniPageDefault = 50  // righe per pagina se Page.Limit == 0
	maxMessaggiScansionati   = 500 // tetto di messaggi letti per chiamata
	chunkScansione           = 100 // UID per FETCH durante la scansione
	threadPerSearch          = 20  // thread per SEARCH OR su All Mail
)

var errGmailNonDisponibile = errors.New("imap: estensioni Gmail (X-GM-EXT-1) non disponibili su questo server")

// ListConversations implementa mail.MailStore (Piano 8, Task 7).
func (s *Store) ListConversations(ctx context.Context, q msg.Query, pg mail.Page) ([]mail.Conversation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !s.gmailOn() {
		if q.Raw != "" {
			return nil, errGmailNonDisponibile
		}
		refs, err := s.ListMessages(ctx, q, pg)
		if err != nil {
			return nil, err
		}
		out := make([]mail.Conversation, 0, len(refs))
		for _, r := range refs {
			out = append(out, singleConversation(r))
		}
		return out, nil
	}

	// allMailName prende il gate (ListMailboxes): va chiamata PRIMA di acquire.
	allMail, err := s.allMailName(ctx)
	if err != nil {
		return nil, err
	}
	mailbox := q.Label
	if mailbox == "" {
		mailbox = "INBOX"
	}
	crit := searchCriteria(q)
	if q.Raw != "" {
		// D8-4: senza label: la ricerca Gmail è su All Mail, come Gmail web.
		crit = &imap.SearchCriteria{Gmail: &imap.SearchCriteriaGmail{Raw: q.Raw}}
		if q.Label == "" {
			mailbox = allMail
		}
	}

	defer s.gate.acquire()()
	c, err := s.connected()
	if err != nil {
		return nil, err
	}
	if _, err := c.Select(mailbox, nil).Wait(); err != nil {
		return nil, fmt.Errorf("imap select %s: %w", mailbox, err)
	}
	data, err := c.UIDSearch(crit, nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("imap uid search in %s: %w", mailbox, err)
	}
	uids := data.AllUIDs()

	limit := pg.Limit
	if limit == 0 {
		limit = conversazioniPageDefault
	}
	offset := pg.Offset

	// Scansione dal più recente: un messaggio per thread (il più recente).
	var latest []msg.MessageRef
	seen := make(map[string]bool)
	scanned := 0
	for i := len(uids) - 1; i >= 0 && len(latest) < offset+limit && scanned < maxMessaggiScansionati; {
		lo := i - chunkScansione + 1
		if lo < 0 {
			lo = 0
		}
		chunk := uids[lo : i+1]
		scanned += len(chunk)
		refs, err := gmailRefs(c, mailbox, chunk)
		if err != nil {
			return nil, err
		}
		for _, r := range refs {
			key := threadKey(r)
			if seen[key] {
				continue
			}
			seen[key] = true
			latest = append(latest, r)
			if len(latest) >= offset+limit {
				break
			}
		}
		i = lo - 1
	}

	if offset >= len(latest) {
		return nil, nil
	}
	window := latest[offset:]
	if len(window) > limit {
		window = window[:limit]
	}

	var threads []uint64
	for _, r := range window {
		if r.ThreadID != 0 {
			threads = append(threads, r.ThreadID)
		}
	}
	sums, err := threadSummaries(c, allMail, threads)
	if err != nil {
		return nil, err
	}
	out := make([]mail.Conversation, 0, len(window))
	for _, r := range window {
		if r.ThreadID == 0 {
			out = append(out, singleConversation(r))
			continue
		}
		out = append(out, summaryConversation(r, sums[r.ThreadID]))
	}
	return out, nil
}

// ThreadMessages implementa mail.MailStore (Piano 8, Task 7): i messaggi del
// thread su All Mail, cronologici. Spam e cestino non compaiono in All Mail.
func (s *Store) ThreadMessages(ctx context.Context, threadID uint64) ([]msg.MessageRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if threadID == 0 {
		return nil, errors.New("imap: thread 0 non valido")
	}
	if !s.gmailOn() {
		return nil, errGmailNonDisponibile
	}
	allMail, err := s.allMailName(ctx)
	if err != nil {
		return nil, err
	}
	defer s.gate.acquire()()
	c, err := s.connected()
	if err != nil {
		return nil, err
	}
	if _, err := c.Select(allMail, nil).Wait(); err != nil {
		return nil, fmt.Errorf("imap select %s: %w", allMail, err)
	}
	data, err := c.UIDSearch(&imap.SearchCriteria{Gmail: &imap.SearchCriteriaGmail{ThreadID: threadID}}, nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("imap thread search: %w", err)
	}
	refs, err := gmailRefs(c, allMail, data.AllUIDs())
	if err != nil {
		return nil, err
	}
	sort.SliceStable(refs, func(i, j int) bool { return refs[i].Date.Before(refs[j].Date) })
	return refs, nil
}

// allMailName restituisce il nome della cartella con ruolo "all" (All Mail,
// SPECIAL-USE). Prende il gate tramite ListMailboxes: non chiamarla con il gate.
func (s *Store) allMailName(ctx context.Context) (string, error) {
	boxes, err := s.ListMailboxes(ctx)
	if err != nil {
		return "", err
	}
	for _, b := range boxes {
		if b.Role == "all" {
			return b.Name, nil
		}
	}
	return "", errors.New("imap: cartella Tutti i messaggi (\\All) non trovata")
}

// gmailRefs scarica ENVELOPE, FLAGS, INTERNALDATE e X-GM-* per gli UID della
// mailbox già selezionata, dal più recente al più vecchio. Gate già detenuto.
func gmailRefs(c *imapclient.Client, mailbox string, uids []imap.UID) ([]msg.MessageRef, error) {
	if len(uids) == 0 {
		return nil, nil
	}
	bufs, err := c.Fetch(imap.UIDSetNum(uids...), &imap.FetchOptions{
		UID:           true,
		Envelope:      true,
		Flags:         true,
		InternalDate:  true,
		GmailMsgID:    true,
		GmailThreadID: true,
		GmailLabels:   true,
	}).Collect()
	if err != nil {
		return nil, fmt.Errorf("imap fetch gmail in %s: %w", mailbox, err)
	}
	byUID := make(map[imap.UID]*imapclient.FetchMessageBuffer, len(bufs))
	for _, b := range bufs {
		byUID[b.UID] = b
	}
	refs := make([]msg.MessageRef, 0, len(uids))
	for i := len(uids) - 1; i >= 0; i-- {
		buf := byUID[uids[i]]
		ref := toRef(mailbox, uids[i], buf)
		if buf != nil {
			ref.GmailID = buf.GmailMsgID
			ref.ThreadID = buf.GmailThreadID
			ref.Labels = userLabels(buf.GmailLabels)
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

// threadSummaries calcola Count, Participants, Unread e Labels per thread su
// All Mail, con SEARCH OR di threadPerSearch thread alla volta. Gate detenuto.
func threadSummaries(c *imapclient.Client, allMail string, threads []uint64) (map[uint64]threadSummary, error) {
	out := make(map[uint64]threadSummary, len(threads))
	if len(threads) == 0 {
		return out, nil
	}
	if _, err := c.Select(allMail, nil).Wait(); err != nil {
		return nil, fmt.Errorf("imap select %s: %w", allMail, err)
	}
	groups := make(map[uint64][]msg.MessageRef)
	for start := 0; start < len(threads); start += threadPerSearch {
		end := start + threadPerSearch
		if end > len(threads) {
			end = len(threads)
		}
		group := threads[start:end]
		data, err := c.UIDSearch(threadCriteria(group), nil).Wait()
		if err != nil {
			return nil, fmt.Errorf("imap thread summary search: %w", err)
		}
		refs, err := gmailRefs(c, allMail, data.AllUIDs())
		if err != nil {
			return nil, err
		}
		for _, r := range refs {
			groups[r.ThreadID] = append(groups[r.ThreadID], r)
		}
	}
	for id, refs := range groups {
		out[id] = summarizeThread(refs)
	}
	return out, nil
}

// threadCriteria costruisce OR(X-GM-THRID a, X-GM-THRID b, ...) come catena
// annidata: IMAP SEARCH non ha un OR n-ario.
func threadCriteria(ids []uint64) *imap.SearchCriteria {
	var crit *imap.SearchCriteria
	for _, id := range ids {
		leaf := imap.SearchCriteria{Gmail: &imap.SearchCriteriaGmail{ThreadID: id}}
		if crit == nil {
			crit = &leaf
			continue
		}
		crit = &imap.SearchCriteria{Or: [][2]imap.SearchCriteria{{*crit, leaf}}}
	}
	if crit == nil {
		crit = &imap.SearchCriteria{}
	}
	return crit
}

// threadSummary è l'aggregato di un thread su All Mail.
type threadSummary struct {
	count        int
	participants []string
	unread       bool
	labels       []string
}

// summarizeThread aggrega i messaggi di un thread (in qualunque ordine).
func summarizeThread(refs []msg.MessageRef) threadSummary {
	sorted := append([]msg.MessageRef(nil), refs...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Date.Before(sorted[j].Date) })
	sum := threadSummary{count: len(sorted)}
	seenFrom := make(map[string]bool)
	seenLabel := make(map[string]bool)
	for _, r := range sorted {
		if r.Unread {
			sum.unread = true
		}
		if r.From != "" && !seenFrom[r.From] {
			seenFrom[r.From] = true
			sum.participants = append(sum.participants, r.From)
		}
		for _, l := range r.Labels {
			if !seenLabel[l] {
				seenLabel[l] = true
				sum.labels = append(sum.labels, l)
			}
		}
	}
	return sum
}

// summaryConversation compone la riga: Latest è il messaggio più recente nella
// mailbox richiesta, il resto viene dal riepilogo su All Mail.
func summaryConversation(latest msg.MessageRef, sum threadSummary) mail.Conversation {
	return mail.Conversation{
		Latest:       latest,
		ThreadID:     latest.ThreadID,
		Count:        sum.count,
		Participants: sum.participants,
		Unread:       sum.unread,
		Labels:       sum.labels,
	}
}

// singleConversation è la riga di un messaggio senza thread (server non Gmail).
func singleConversation(r msg.MessageRef) mail.Conversation {
	c := mail.Conversation{Latest: r, ThreadID: r.ThreadID, Count: 1, Unread: r.Unread, Labels: r.Labels}
	if r.From != "" {
		c.Participants = []string{r.From}
	}
	return c
}

// threadKey raggruppa i messaggi: il thread, o il messaggio se non Gmail.
func threadKey(r msg.MessageRef) string {
	if r.ThreadID != 0 {
		return fmt.Sprintf("thread:%d", r.ThreadID)
	}
	return "msg:" + string(r.ID)
}

// userLabels filtra le label di sistema (\Inbox, \Sent, Important, Starred,
// Chats): restano le label utente.
func userLabels(labels []string) []string {
	var out []string
	for _, l := range labels {
		if strings.HasPrefix(l, "\\") || l == "Important" || l == "Starred" || l == "Chats" {
			continue
		}
		out = append(out, l)
	}
	return out
}
