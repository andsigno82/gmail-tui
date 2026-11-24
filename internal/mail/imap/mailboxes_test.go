package imap

// Test di ListMailboxes (Piano 7, Task 1): ruoli da SPECIAL-USE, scarto di
// \Noselect, ordine di presentazione. Gli attributi passano da
// createSpecialMailbox/listAs (cfr. imap_test.go: il memserver beta.9 non
// emette CreateOptions.SpecialUse).

import (
	"testing"

	"github.com/emersion/go-imap/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// gmailCaps sono le capability che Gmail annuncia e che il Trash richiede
// (MOVE, UIDPLUS), più quelle per LIST RETURN (SPECIAL-USE).
var gmailCaps = imap.CapSet{
	imap.CapIMAP4rev1:    {},
	imap.CapMove:         {},
	imap.CapUIDPlus:      {},
	imap.CapSpecialUse:   {},
	imap.CapListExtended: {},
}

// mailboxSubjects elenca i subject della mailbox (Label vuota = INBOX), nel
// formato di ListMessages (dal più nuovo).
func mailboxSubjects(t *testing.T, store *Store, mailbox string) []string {
	t.Helper()
	refs, err := store.ListMessages(t.Context(), msg.Query{Label: mailbox}, mail.Page{})
	if err != nil {
		t.Fatalf("ListMessages %q: %v", mailbox, err)
	}
	return subjects(refs)
}

// TestListMailboxesRolesOrderAndNoselect: ruoli SPECIAL-USE mappati, il
// contenitore \Noselect "[Gmail]" scartato, ordine INBOX, speciali nell'ordine
// fisso (Importanti, Speciali, Inviati, Bozze, Tutti, Spam, Cestino), poi le label in
// ordine alfabetico.
func TestListMailboxesRolesOrderAndNoselect(t *testing.T) {
	ts := startTestServerCaps(t, gmailCaps)
	ts.listAs("[Gmail]", imap.MailboxAttrNoSelect)
	ts.createSpecialMailbox("[Gmail]/Trash", imap.MailboxAttrTrash)
	ts.createSpecialMailbox("[Gmail]/Important", imap.MailboxAttrImportant) // Piano 12a, Task 6
	ts.createSpecialMailbox("[Gmail]/Sent Mail", imap.MailboxAttrSent)
	ts.createSpecialMailbox("[Gmail]/Drafts", imap.MailboxAttrDrafts)
	ts.createSpecialMailbox("[Gmail]/All Mail", imap.MailboxAttrAll)
	ts.createSpecialMailbox("[Gmail]/Spam", imap.MailboxAttrJunk)
	ts.createSpecialMailbox("[Gmail]/Starred", imap.MailboxAttrFlagged)
	ts.createMailbox("Work")
	ts.createMailbox("Archive")
	store := connectStore(t, ts)

	got, err := store.ListMailboxes(t.Context())
	if err != nil {
		t.Fatalf("ListMailboxes: %v", err)
	}
	want := []mail.Mailbox{
		{Name: "INBOX", Role: "inbox", Delimiter: "/"},
		{Name: "[Gmail]/Important", Role: "important", Delimiter: "/"},
		{Name: "[Gmail]/Starred", Role: "flagged", Delimiter: "/"},
		{Name: "[Gmail]/Sent Mail", Role: "sent", Delimiter: "/"},
		{Name: "[Gmail]/Drafts", Role: "drafts", Delimiter: "/"},
		{Name: "[Gmail]/All Mail", Role: "all", Delimiter: "/"},
		{Name: "[Gmail]/Spam", Role: "junk", Delimiter: "/"},
		{Name: "[Gmail]/Trash", Role: "trash", Delimiter: "/"},
		{Name: "Archive", Role: "", Delimiter: "/"},
		{Name: "Work", Role: "", Delimiter: "/"},
	}
	if len(got) != len(want) {
		t.Fatalf("ListMailboxes = %+v, voglio %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ListMailboxes[%d] = %+v, voglio %+v", i, got[i], want[i])
		}
	}
}

// TestListMailboxesDelimiter: il delimitatore del LIST arriva in Mailbox.Delimiter
// (Piano 15, Task 11), anche per una label annidata.
func TestListMailboxesDelimiter(t *testing.T) {
	ts := startTestServerCaps(t, gmailCaps)
	ts.createMailbox("Work")
	ts.createMailbox("Work/Sub")
	store := connectStore(t, ts)

	got, err := store.ListMailboxes(t.Context())
	if err != nil {
		t.Fatalf("ListMailboxes: %v", err)
	}
	want := []mail.Mailbox{
		{Name: "INBOX", Role: "inbox", Delimiter: "/"},
		{Name: "Work", Role: "", Delimiter: "/"},
		{Name: "Work/Sub", Role: "", Delimiter: "/"},
	}
	if len(got) != len(want) {
		t.Fatalf("ListMailboxes = %+v, voglio %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ListMailboxes[%d] = %+v, voglio %+v", i, got[i], want[i])
		}
	}
}
