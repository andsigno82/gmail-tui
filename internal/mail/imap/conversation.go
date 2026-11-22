package imap

// Piano 8, Task 5: stub dei metodi di conversazione del contratto MailStore.
// L'implementazione reale (X-GM-THRID) arriva nel Task 7: fino ad allora il
// backend risponde con errore esplicito e nessuna chiamata di rete.

import (
	"context"
	"errors"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

var errConversazioniNonImplementate = errors.New("imap: conversazioni Gmail non ancora implementate (Piano 8, Task 7)")

// ListConversations implementa mail.MailStore (stub, Piano 8, Task 5).
func (s *Store) ListConversations(context.Context, msg.Query, mail.Page) ([]mail.Conversation, error) {
	return nil, errConversazioniNonImplementate
}

// ThreadMessages implementa mail.MailStore (stub, Piano 8, Task 5).
func (s *Store) ThreadMessages(context.Context, uint64) ([]msg.MessageRef, error) {
	return nil, errConversazioniNonImplementate
}
