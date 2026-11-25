package imap

// Misura dei passi IMAP (Piano 12a, Task 5, D12-T3): ogni comando del percorso di
// LETTURA emette un evento "step" della traccia. Il nome dell'operazione viene dal
// ctx: lo mette il wrapper della traccia (trace.WithSpan), oppure trace.WithOp per
// i comandi del Watch e per l'apertura della sessione. Il ruolo della cartella
// arriva dall'ultimo LIST: mai il nome. Le mutazioni (STORE, MOVE, APPEND) non si
// misurano. Senza traccia installata ogni funzione qui e una chiamata vuota.

import (
	"context"
	"fmt"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/trace"
)

// noteRoles ricorda il ruolo di ogni mailbox dell'ultimo LIST (Piano 12a, Task 5).
func (s *Store) noteRoles(boxes []mail.Mailbox) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.roles == nil {
		s.roles = make(map[string]string, len(boxes))
	}
	for _, b := range boxes {
		s.roles[b.Name] = b.Role
	}
}

// traceRole e il ruolo da mettere nella traccia per una mailbox: "inbox" per INBOX,
// il ruolo dell'ultimo LIST se noto, altrimenti "label". Mai il nome.
func (s *Store) traceRole(mailbox string) string {
	if mailbox == "" || strings.EqualFold(mailbox, "INBOX") {
		return "inbox"
	}
	s.mu.Lock()
	role := s.roles[mailbox]
	s.mu.Unlock()
	if role == "" {
		return "label"
	}
	return role
}

// selectBox e la SELECT del percorso di lettura: misura il passo "select" (N = i
// messaggi della mailbox) e registra l'epoca (Piano 16, R3), come facevano i chiamanti
// di lista, thread e corpi. Fa eccezione la SELECT che idleRound fa prima di ogni
// IDLE, che prima non la registrava: registrarla anche li e innocuo, perche
// recheckNewMail registra la stessa epoca subito dopo il round. Gate gia detenuto.
func (s *Store) selectBox(ctx context.Context, c *imapclient.Client, mailbox string) (*imap.SelectData, error) {
	step := trace.BeginStep(ctx, trace.StepSelect)
	sel, err := c.Select(mailbox, nil).Wait()
	if err != nil {
		step.End(s.traceRole(mailbox), 0, 0, err)
		return nil, fmt.Errorf("imap select %s: %w", mailbox, err)
	}
	s.noteEpoch(mailbox, sel.UIDValidity)
	step.End(s.traceRole(mailbox), int(sel.NumMessages), 0, nil)
	return sel, nil
}

// uidSearch e la UID SEARCH del percorso di lettura: misura il passo "search"
// (N = gli UID restituiti). L'errore arriva nudo: il testo lo aggiunge il chiamante.
// Gate gia detenuto.
func (s *Store) uidSearch(ctx context.Context, c *imapclient.Client, mailbox string, crit *imap.SearchCriteria, opts *imap.SearchOptions) (*imap.SearchData, error) {
	step := trace.BeginStep(ctx, trace.StepSearch)
	data, err := c.UIDSearch(crit, opts).Wait()
	if err != nil {
		step.End(s.traceRole(mailbox), 0, 0, err)
		return nil, err
	}
	step.End(s.traceRole(mailbox), len(data.AllUIDs()), 0, nil)
	return data, nil
}
