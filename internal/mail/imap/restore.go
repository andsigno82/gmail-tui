package imap

// Piano 10, Task 4: stub di Restore (MOVE di ritorno dal cestino). La versione reale
// (Task 5, D10-T3) sostituisce questo file.

import (
	"context"
	"errors"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

var errNonImplementatoPiano10 = errors.New("imap: non ancora implementato (Piano 10, Task 5)")

// Restore implementa mail.MailStore (stub, Piano 10, Task 4).
func (s *Store) Restore(context.Context, []msg.MessageID, string) ([]msg.MessageID, error) {
	return nil, errNonImplementatoPiano10
}
