// Package imap implementa mail.MailStore su un server IMAP (Gmail) via
// emersion/go-imap/v2.
package imap

import (
	"github.com/emersion/go-imap/v2"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// searchCriteria traduce msg.Query in criteri IMAP SEARCH (funzione pura,
// nessuna I/O). Campi multipli = intersezione (AND) lato server.
//
// Tabella di traduzione:
//   - From, To → ricerca su header (HEADER From/To);
//   - Text → TEXT: corpi e subject lato server;
//   - Unread → messaggi SENZA flag \Seen (NotFlag);
//   - Starred → messaggi CON flag \Flagged.
//
// Query vuota → SearchCriteria zero-value: nessun criterio = match-all
// (SEARCH senza argomenti equivale ad ALL).
//
// NON traducibili in SEARCH e volutamente ignorati qui:
//   - Label: seleziona la mailbox su cui fare Select/Search (Task 3/6);
//   - HasAttachment: IMAP SEARCH non ha criteri allegati; best-effort
//     come post-filtro client-side (Task 6).
func searchCriteria(q msg.Query) *imap.SearchCriteria {
	c := &imap.SearchCriteria{}
	if q.From != "" {
		c.Header = append(c.Header, imap.SearchCriteriaHeaderField{Key: "From", Value: q.From})
	}
	if q.To != "" {
		c.Header = append(c.Header, imap.SearchCriteriaHeaderField{Key: "To", Value: q.To})
	}
	if q.Text != "" {
		c.Text = append(c.Text, q.Text)
	}
	if q.Unread {
		c.NotFlag = append(c.NotFlag, imap.FlagSeen)
	}
	if q.Starred {
		c.Flag = append(c.Flag, imap.FlagFlagged)
	}
	return c
}
