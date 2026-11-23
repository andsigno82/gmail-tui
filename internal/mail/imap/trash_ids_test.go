package imap

// Piano 10, Task 5: Trash restituisce gli ID nel cestino (MoveData.DestUIDs), base
// dell'annulla del cestino (D10-T2). Memserver con MOVE, poi con solo UIDPLUS.

import (
	"testing"

	"github.com/emersion/go-imap/v2"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// TestTrashReturnsNewIDs: con MOVE, l'ID restituito è quello del messaggio nel cestino.
func TestTrashReturnsNewIDs(t *testing.T) {
	ts := startTestServerCaps(t, gmailCaps)
	ts.createSpecialMailbox("[Gmail]/Trash", imap.MailboxAttrTrash)
	uid := ts.seedMail("In <i@example.org>", "In inbox", "corpo", false, false)
	ts.seedMail("Keep <k@example.org>", "Keep", "corpo", false, false)
	store := connectStore(t, ts)

	got, err := store.Trash(t.Context(), []msg.MessageID{msg.NewID("INBOX", uid)})
	if err != nil {
		t.Fatalf("Trash: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Trash restituisce %v, voglio 1 ID", got)
	}
	if box, _, err := got[0].Split(); err != nil || box != "[Gmail]/Trash" {
		t.Errorf("ID restituito %q, voglio un ID nel cestino", got[0])
	}
	if subs := mailboxSubjects(t, store, "[Gmail]/Trash"); len(subs) != 1 || subs[0] != "In inbox" {
		t.Errorf("cestino = %v, voglio [In inbox]", subs)
	}
}

// TestTrashWithUIDPlusOnlyReturnsNewIDs: senza MOVE, COPY + UID EXPUNGE di UIDPLUS
// riporta ancora gli ID nel cestino.
func TestTrashWithUIDPlusOnlyReturnsNewIDs(t *testing.T) {
	ts := startTestServerCaps(t, imap.CapSet{
		imap.CapIMAP4rev1:  {},
		imap.CapUIDPlus:    {},
		imap.CapSpecialUse: {},
	})
	ts.createSpecialMailbox("[Gmail]/Trash", imap.MailboxAttrTrash)
	uid := ts.seedMail("In <i@example.org>", "In inbox", "corpo", false, false)
	store := connectStore(t, ts)

	got, err := store.Trash(t.Context(), []msg.MessageID{msg.NewID("INBOX", uid)})
	if err != nil {
		t.Fatalf("Trash con solo UIDPLUS: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Trash con solo UIDPLUS restituisce %v, voglio 1 ID", got)
	}
}
