package imap

// Cartelle e label (Piano 7, Task 1): LIST con RETURN (SPECIAL-USE) mappato sui
// ruoli di mail.Mailbox. Il ruolo viene dagli attributi SPECIAL-USE (RFC 6154),
// mai dal nome localizzato ("[Gmail]/Cestino" o "[Gmail]/Trash").

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/trace"
)

// specialRoles associa gli attributi SPECIAL-USE ai ruoli, nell'ordine di
// presentazione (Piano 7, Task 1: Speciali, Inviati, Bozze, Tutti, Spam, Cestino).
var specialRoles = []struct {
	attr imap.MailboxAttr
	role string
}{
	{imap.MailboxAttrFlagged, "flagged"},
	{imap.MailboxAttrSent, "sent"},
	{imap.MailboxAttrDrafts, "drafts"},
	{imap.MailboxAttrAll, "all"},
	{imap.MailboxAttrJunk, "junk"},
	{imap.MailboxAttrTrash, "trash"},
}

// ListMailboxes implementa mail.MailStore (Piano 7, Task 1). Sotto gate.
func (s *Store) ListMailboxes(ctx context.Context) ([]mail.Mailbox, error) {
	defer s.acquire(ctx)()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c, err := s.connected()
	if err != nil {
		return nil, err
	}
	boxes, err := listMailboxes(ctx, c)
	if err != nil {
		return nil, err
	}
	s.noteRoles(boxes) // Piano 12a, Task 5: i ruoli servono alla traccia dei passi
	return boxes, nil
}

// listMailboxes è il corpo di ListMailboxes SENZA gate: il gate non è rientrante
// (cfr. idleGate), quindi chi è già sotto gate (Trash) lo chiama direttamente.
func listMailboxes(ctx context.Context, c *imapclient.Client) ([]mail.Mailbox, error) {
	data, err := listData(ctx, c, nil)
	if err != nil {
		return nil, err
	}
	boxes := make([]mail.Mailbox, 0, len(data))
	for _, d := range data {
		if hasAttr(d.Attrs, imap.MailboxAttrNoSelect) {
			continue // contenitore non selezionabile (es. "[Gmail]")
		}
		boxes = append(boxes, mail.Mailbox{Name: d.Mailbox, Role: roleOf(d), Delimiter: delimOf(d)})
	}
	sort.SliceStable(boxes, func(i, j int) bool {
		ri, rj := roleRank(boxes[i].Role), roleRank(boxes[j].Role)
		if ri != rj {
			return ri < rj
		}
		return boxes[i].Name < boxes[j].Name
	})
	return boxes, nil
}

// listData è il LIST "*" con RETURN (SPECIAL-USE), SENZA gate. status != nil
// aggiunge RETURN (STATUS ...) (LIST-STATUS, RFC 5819: solo se il server lo
// annuncia; Piano 15, Task 11). Condiviso da listMailboxes e unreadCounts.
func listData(ctx context.Context, c *imapclient.Client, status *imap.StatusOptions) ([]*imap.ListData, error) {
	step := trace.BeginStep(ctx, trace.StepList) // Piano 12a, Task 5
	data, err := c.List("", "*", &imap.ListOptions{ReturnSpecialUse: true, ReturnStatus: status}).Collect()
	if err != nil {
		step.End("", 0, 0, err)
		return nil, fmt.Errorf("imap list: %w", err)
	}
	step.End("", len(data), 0, nil)
	return data, nil
}

// delimOf è il delimitatore gerarchico del LIST ("/" per Gmail); vuoto se il
// server risponde NIL (rune 0). Piano 15, Task 11.
func delimOf(d *imap.ListData) string {
	if d.Delim == 0 {
		return ""
	}
	return string(d.Delim)
}

// roleOf ricava il ruolo di una mailbox LIST: INBOX per nome (IMAP: case
// insensitive), altrimenti il primo attributo SPECIAL-USE noto; "" = label.
func roleOf(d *imap.ListData) string {
	if strings.EqualFold(d.Mailbox, "INBOX") {
		return "inbox"
	}
	for _, sr := range specialRoles {
		if hasAttr(d.Attrs, sr.attr) {
			return sr.role
		}
	}
	return ""
}

// roleRank dà la posizione di presentazione di un ruolo: INBOX prima, poi le
// speciali nell'ordine di specialRoles, poi le label.
func roleRank(role string) int {
	if role == "inbox" {
		return 0
	}
	for i, sr := range specialRoles {
		if sr.role == role {
			return i + 1
		}
	}
	return len(specialRoles) + 1
}

// hasAttr dice se gli attributi contengono want.
func hasAttr(attrs []imap.MailboxAttr, want imap.MailboxAttr) bool {
	for _, a := range attrs {
		if a == want {
			return true
		}
	}
	return false
}
