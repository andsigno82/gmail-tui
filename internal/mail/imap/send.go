package imap

import (
	"context"
	"errors"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Send implementa MailStore, ma l'invio NON è una capacità IMAP: la
// conversazione SMTP (dial → STARTTLS → AUTH → MAIL/RCPT/DATA) è di
// smtp.Sender (Piano 2 Task 8), che il chiamante usa direttamente
// (cfr. cmd.Send). Il metodo esiste solo per completare il contratto
// MailStore di Piano 1 (assert di conformenza su Store): ritorna sempre
// errore con l'indicazione del percorso giusto.
func (s *Store) Send(context.Context, msg.Outgoing) error {
	return errors.New("imap: l'invio avviene via SMTP — usare smtp.Sender")
}
