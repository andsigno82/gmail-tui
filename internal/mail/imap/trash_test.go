package imap

// Test di Trash (Piano 7, Task 1): UID MOVE nella mailbox con ruolo trash.
// Casi di rifiuto: nessun cestino, id già nel cestino, server senza MOVE né
// UIDPLUS (mai EXPUNGE generico). La harness è in imap_test.go.

import (
	"testing"

	"github.com/emersion/go-imap/v2"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// TestTrashMovesFromInboxAndLabel: Trash su id di due mailbox (INBOX e label
// Work) → entrambi escono dalla cartella d'origine ed entrano nel cestino.
func TestTrashMovesFromInboxAndLabel(t *testing.T) {
	ts := startTestServerCaps(t, gmailCaps)
	ts.createSpecialMailbox("[Gmail]/Trash", imap.MailboxAttrTrash)
	ts.createMailbox("Work")
	uidIn := ts.seedMail("In <i@example.org>", "In inbox", "corpo", false, false)
	uidWk := ts.seedInMailbox("Work", "W <w@example.org>", "In work", "corpo", false, false)
	ts.seedMail("Keep <k@example.org>", "Keep", "corpo", false, false)
	store := connectStore(t, ts)

	if err := store.Trash(t.Context(), []msg.MessageID{
		msg.NewID("INBOX", uidIn),
		msg.NewID("Work", uidWk),
	}); err != nil {
		t.Fatalf("Trash: %v", err)
	}

	if got := mailboxSubjects(t, store, ""); len(got) != 1 || got[0] != "Keep" {
		t.Errorf("INBOX dopo Trash = %v, voglio [Keep]", got)
	}
	if got := mailboxSubjects(t, store, "Work"); len(got) != 0 {
		t.Errorf("Work dopo Trash = %v, voglio vuota", got)
	}
	if got := mailboxSubjects(t, store, "[Gmail]/Trash"); len(got) != 2 {
		t.Errorf("cestino dopo Trash = %v, voglio 2 messaggi", got)
	}
}

// TestTrashWithoutTrashMailboxFails: nessuna mailbox \Trash → errore e
// nessun messaggio toccato.
func TestTrashWithoutTrashMailboxFails(t *testing.T) {
	ts := startTestServerCaps(t, gmailCaps)
	uid := ts.seedMail("In <i@example.org>", "In inbox", "corpo", false, false)
	store := connectStore(t, ts)

	if err := store.Trash(t.Context(), []msg.MessageID{msg.NewID("INBOX", uid)}); err == nil {
		t.Fatal("Trash senza cestino: err = nil, voglio errore")
	}
	if got := mailboxSubjects(t, store, ""); len(got) != 1 {
		t.Errorf("INBOX dopo Trash fallito = %v, voglio il messaggio ancora lì", got)
	}
}

// TestTrashRejectsIDAlreadyInTrash: un id già nel cestino fa rifiutare tutta
// la chiamata: il messaggio in INBOX resta dov'è.
func TestTrashRejectsIDAlreadyInTrash(t *testing.T) {
	ts := startTestServerCaps(t, gmailCaps)
	ts.createSpecialMailbox("[Gmail]/Trash", imap.MailboxAttrTrash)
	uidIn := ts.seedMail("In <i@example.org>", "In inbox", "corpo", false, false)
	uidTr := ts.seedInMailbox("[Gmail]/Trash", "T <t@example.org>", "In trash", "corpo", false, false)
	store := connectStore(t, ts)

	if err := store.Trash(t.Context(), []msg.MessageID{
		msg.NewID("INBOX", uidIn),
		msg.NewID("[Gmail]/Trash", uidTr),
	}); err == nil {
		t.Fatal("Trash con id nel cestino: err = nil, voglio errore")
	}
	if got := mailboxSubjects(t, store, ""); len(got) != 1 || got[0] != "In inbox" {
		t.Errorf("INBOX = %v, voglio ancora [In inbox] (nessuna azione)", got)
	}
	if got := mailboxSubjects(t, store, "[Gmail]/Trash"); len(got) != 1 {
		t.Errorf("cestino = %v, voglio invariato (1 messaggio)", got)
	}
}

// TestTrashRefusesWithoutMoveOrUIDPlus: server senza MOVE né UIDPLUS →
// rifiuto prima di qualsiasi azione (il fallback di Move userebbe EXPUNGE
// generico).
func TestTrashRefusesWithoutMoveOrUIDPlus(t *testing.T) {
	ts := startTestServerCaps(t, imap.CapSet{
		imap.CapIMAP4rev1:  {},
		imap.CapSpecialUse: {},
	})
	ts.createSpecialMailbox("[Gmail]/Trash", imap.MailboxAttrTrash)
	uid := ts.seedMail("In <i@example.org>", "In inbox", "corpo", false, false)
	store := connectStore(t, ts)

	if err := store.Trash(t.Context(), []msg.MessageID{msg.NewID("INBOX", uid)}); err == nil {
		t.Fatal("Trash senza MOVE né UIDPLUS: err = nil, voglio rifiuto")
	}
	if got := mailboxSubjects(t, store, ""); len(got) != 1 {
		t.Errorf("INBOX dopo rifiuto = %v, voglio il messaggio ancora lì", got)
	}
}

// TestTrashWithUIDPlusOnly: server con solo UIDPLUS (niente MOVE) → Trash
// riesce via COPY + STORE + UID EXPUNGE, senza EXPUNGE generico.
func TestTrashWithUIDPlusOnly(t *testing.T) {
	ts := startTestServerCaps(t, imap.CapSet{
		imap.CapIMAP4rev1:  {},
		imap.CapUIDPlus:    {},
		imap.CapSpecialUse: {},
	})
	ts.createSpecialMailbox("[Gmail]/Trash", imap.MailboxAttrTrash)
	uidIn := ts.seedMail("In <i@example.org>", "In inbox", "corpo", false, false)
	ts.seedMail("Keep <k@example.org>", "Keep", "corpo", false, false)
	store := connectStore(t, ts)

	if err := store.Trash(t.Context(), []msg.MessageID{msg.NewID("INBOX", uidIn)}); err != nil {
		t.Fatalf("Trash con solo UIDPLUS: %v", err)
	}
	if got := mailboxSubjects(t, store, ""); len(got) != 1 || got[0] != "Keep" {
		t.Errorf("INBOX dopo Trash = %v, voglio [Keep]", got)
	}
	if got := mailboxSubjects(t, store, "[Gmail]/Trash"); len(got) != 1 {
		t.Errorf("cestino dopo Trash = %v, voglio 1 messaggio", got)
	}
}
