package imap

import (
	"testing"

	"github.com/emersion/go-imap/v2"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Piano 8, Task 9b (test mancante): Archive e Trash rifiutano i messaggi di
// [Gmail]/Tutti i messaggi (ruolo "all"). Lì l'expunge o il MOVE li
// cancellerebbero dalla posta. Il memserver non assegna \All da solo: la mailbox
// si crea con createSpecialMailbox (attributi SPECIAL-USE serviti dal LIST).
func TestArchiveETrashRifiutanoTuttiIMessaggi(t *testing.T) {
	ts := startTestServerCaps(t, gmailCaps)
	ts.createSpecialMailbox("[Gmail]/Trash", imap.MailboxAttrTrash)
	ts.createSpecialMailbox("[Gmail]/All Mail", imap.MailboxAttrAll)
	uid := ts.seedInMailbox("[Gmail]/All Mail", "A <a@example.org>", "In tutti", "corpo", false, false)
	store := connectStore(t, ts)
	id := msg.NewID("[Gmail]/All Mail", uid)

	if err := store.Archive(t.Context(), []msg.MessageID{id}); err == nil {
		t.Error("Archive su Tutti i messaggi deve rifiutare")
	}
	if err := store.Trash(t.Context(), []msg.MessageID{id}); err == nil {
		t.Error("Trash su Tutti i messaggi deve rifiutare")
	}
	if got := mailboxSubjects(t, store, "[Gmail]/All Mail"); len(got) != 1 {
		t.Errorf("Tutti i messaggi dopo i rifiuti = %v, voglio il messaggio intatto", got)
	}
	if got := mailboxSubjects(t, store, "[Gmail]/Trash"); len(got) != 0 {
		t.Errorf("cestino dopo il rifiuto = %v, voglio vuoto", got)
	}
}
