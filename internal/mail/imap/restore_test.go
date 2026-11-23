package imap

// Piano 10, Task 5: Restore riporta i messaggi dal cestino alla cartella d'origine
// con MOVE e UID (D10-T3). Guard prima di ogni comando: ID fuori dal cestino e
// destinazione uguale al cestino sono rifiutati.

import (
	"testing"

	"github.com/emersion/go-imap/v2"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// TestRestoreMovesBackFromTrash: l'ID nel cestino torna in INBOX con un nuovo UID.
func TestRestoreMovesBackFromTrash(t *testing.T) {
	ts := startTestServerCaps(t, gmailCaps)
	ts.createSpecialMailbox("[Gmail]/Trash", imap.MailboxAttrTrash)
	ts.seedMail("Keep <k@example.org>", "Keep", "corpo", false, false)
	uidTr := ts.seedInMailbox("[Gmail]/Trash", "T <t@example.org>", "Nel cestino", "corpo", false, false)
	store := connectStore(t, ts)

	got, err := store.Restore(t.Context(), []msg.MessageID{msg.NewID("[Gmail]/Trash", uidTr)}, "INBOX")
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Restore restituisce %v, voglio 1 ID", got)
	}
	if box, _, err := got[0].Split(); err != nil || box != "INBOX" {
		t.Errorf("ID restituito %q, voglio un ID in INBOX", got[0])
	}
	if subs := mailboxSubjects(t, store, ""); len(subs) != 2 {
		t.Errorf("INBOX dopo Restore = %v, voglio 2 messaggi", subs)
	}
	if subs := mailboxSubjects(t, store, "[Gmail]/Trash"); len(subs) != 0 {
		t.Errorf("cestino dopo Restore = %v, voglio vuoto", subs)
	}
}

// TestRestoreRefusesNonTrashIDs: un ID in INBOX, o una destinazione uguale al
// cestino, rifiutano senza toccare il messaggio.
func TestRestoreRefusesNonTrashIDs(t *testing.T) {
	ts := startTestServerCaps(t, gmailCaps)
	ts.createSpecialMailbox("[Gmail]/Trash", imap.MailboxAttrTrash)
	uid := ts.seedMail("In <i@example.org>", "In inbox", "corpo", false, false)
	store := connectStore(t, ts)

	if _, err := store.Restore(t.Context(), []msg.MessageID{msg.NewID("INBOX", uid)}, "INBOX"); err == nil {
		t.Fatal("Restore di un ID in INBOX: err = nil, voglio rifiuto")
	}
	if _, err := store.Restore(t.Context(), []msg.MessageID{msg.NewID("INBOX", uid)}, "[Gmail]/Trash"); err == nil {
		t.Fatal("Restore verso il cestino: err = nil, voglio rifiuto")
	}
	if subs := mailboxSubjects(t, store, ""); len(subs) != 1 || subs[0] != "In inbox" {
		t.Errorf("INBOX dopo i rifiuti = %v, voglio ancora [In inbox]", subs)
	}
}
