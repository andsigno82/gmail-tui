package imap

// Contatori dei non letti per cartella (Piano 15, Task 2: solo la firma; la
// vera implementazione STATUS (UNSEEN) sotto gate è nel Task 11).

import (
	"context"
	"errors"
)

// UnreadCounts implementa mail.MailStore (Piano 15, Task 2): stub, sostituito
// nel Task 11.
func (s *Store) UnreadCounts(_ context.Context) (map[string]int, error) {
	return nil, errors.New("non implementato")
}
