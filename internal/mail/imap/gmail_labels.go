package imap

// Piano 10, Task 4: stub di AllMailIDs e ModifyLabels (label Gmail via STORE
// X-GM-LABELS). La versione reale (Task 5, D10-T4/T5) sostituisce questo file.

import (
	"context"
	"errors"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

var errLabelsNonImplementatoPiano10 = errors.New("imap: label Gmail non ancora implementate (Piano 10, Task 5)")

// AllMailIDs implementa mail.MailStore (stub, Piano 10, Task 4).
func (s *Store) AllMailIDs(context.Context, []msg.MessageID) ([]msg.MessageID, error) {
	return nil, errLabelsNonImplementatoPiano10
}

// ModifyLabels implementa mail.MailStore (stub, Piano 10, Task 4).
func (s *Store) ModifyLabels(context.Context, []msg.MessageID, []string, []string) error {
	return errLabelsNonImplementatoPiano10
}
