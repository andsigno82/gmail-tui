package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/theme"
)

// undoFixture: una INBOX con un messaggio (uid 1, GmailID 11) e la sua copia in
// "Tutti i messaggi" (uid 9): Gmail, come serve all'annulla dell'archivia.
// noGmail: stesso fixture senza X-GM-EXT-1 (nessun ritorno da AllMailIDs).
func undoFixture(t *testing.T, noGmail bool) *mail.Mock {
	t.Helper()
	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	mk := func(box string, uid uint32, gid uint64) *msg.Message {
		return &msg.Message{Ref: msg.MessageRef{
			ID: msg.NewID(box, uid), GmailID: gid, From: "anna@x.dev", Subject: "tema", Date: base,
		}}
	}
	return &mail.Mock{
		Mailboxes: []mail.Mailbox{
			{Name: "INBOX", Role: "inbox"},
			{Name: "[Gmail]/Tutti i messaggi", Role: "all"},
			{Name: "[Gmail]/Trash", Role: "trash"},
		},
		Msgs:    []*msg.Message{mk("INBOX", 1, 11), mk("[Gmail]/Tutti i messaggi", 9, 11)},
		NoGmail: noGmail,
	}
}

// toastHasUndo dice se un toast offre `u` (annulla).
func toastHasUndo(m Model) bool {
	for _, s := range m.toast.Texts() {
		if strings.Contains(s, "u annulla") {
			return true
		}
	}
	return false
}

// Piano 10, Task 6 (D10-U1): dopo archivia il toast offre `u`, e `u` riporta il
// messaggio in INBOX (ModifyLabels \Inbox sugli ID di All Mail).
func TestUndoArchiveRestoresInbox(t *testing.T) {
	mock := undoFixture(t, false)
	m := pagedModel(t, mock)
	m, cmd := step(m, pressKey("a"))
	m = applyAll(t, m, cmd)
	if hasInbox(mock, 1) {
		t.Fatal("archivia: il messaggio è ancora in INBOX")
	}
	if !toastHasUndo(m) {
		t.Fatalf("toast dopo archivia = %v, voglio l'offerta di u", m.toast.Texts())
	}
	m, cmd = step(m, pressKey("u"))
	applyAll(t, m, cmd)
	if !hasInbox(mock, 1) {
		t.Error("u: il messaggio non è tornato in INBOX")
	}
}

// Piano 10, Task 6 (D10-T6): senza X-GM-EXT-1 AllMailIDs fallisce e il toast
// NON offre `u`; premere `u` non tocca nulla.
func TestUndoNotOfferedWithoutGmail(t *testing.T) {
	mock := undoFixture(t, true)
	m := pagedModel(t, mock)
	m, cmd := step(m, pressKey("a"))
	m = applyAll(t, m, cmd)
	if toastHasUndo(m) {
		t.Errorf("senza Gmail il toast offre u: %v", m.toast.Texts())
	}
	if m.lastUndo != nil {
		t.Errorf("lastUndo = %+v, voglio nil senza Gmail", m.lastUndo)
	}
	m, cmd = step(m, pressKey("u"))
	m = applyAll(t, m, cmd)
	if hasInbox(mock, 1) {
		t.Error("u senza annulla ha riportato il messaggio in INBOX")
	}
}

// Piano 10, Task 6 (D10-U1, cestino): `#` poi `s` sposta nel cestino; `u` riporta il
// messaggio dal cestino alla INBOX d'origine con Restore sugli ID restituiti da Trash.
func TestUndoTrashRestoresOrigin(t *testing.T) {
	mock := undoFixture(t, false)
	m := pagedModel(t, mock)
	m, _ = step(m, pressKey("#"))
	m, cmd := step(m, pressKey("s"))
	m = applyAll(t, m, cmd)
	if inTrash(mock) != 1 || hasInbox(mock, 1) {
		t.Fatalf("cestino: nel cestino=%d, in INBOX=%v", inTrash(mock), hasInbox(mock, 1))
	}
	if !toastHasUndo(m) {
		t.Fatalf("toast dopo cestino = %v, voglio l'offerta di u", m.toast.Texts())
	}
	m, cmd = step(m, pressKey("u"))
	applyAll(t, m, cmd)
	if inTrash(mock) != 0 || !hasInbox(mock, 1) {
		t.Errorf("u: nel cestino=%d, in INBOX=%v; voglio il messaggio in INBOX", inTrash(mock), hasInbox(mock, 1))
	}
}

// Piano 10, Task 6 (D10-U1): allo scadere dei 10 s (consegnato direttamente con
// seq) l'annulla sparisce: `u` non fa più nulla.
func TestUndoExpiredIgnored(t *testing.T) {
	mock := undoFixture(t, false)
	m := pagedModel(t, mock)
	m, cmd := step(m, pressKey("a"))
	m = applyAll(t, m, cmd)
	if m.lastUndo == nil {
		t.Fatal("precondizione: nessun annulla dopo archivia")
	}
	m, _ = step(m, bulkUndoDueMsg{seq: m.lastUndo.seq + 1}) // seq stantio: ignorato
	if m.lastUndo == nil {
		t.Fatal("un seq stantio ha chiuso l'annulla")
	}
	m, _ = step(m, bulkUndoDueMsg{seq: m.lastUndo.seq})
	if m.lastUndo != nil {
		t.Fatal("scaduta la finestra, l'annulla resta attivo")
	}
	m, cmd = step(m, pressKey("u"))
	m = applyAll(t, m, cmd)
	if hasInbox(mock, 1) {
		t.Error("u dopo la scadenza ha riportato il messaggio in INBOX")
	}
}

// undoSenderModel è la lista di undoFixture con un Sender registrato: serve a
// provare la precedenza fra l'invio in attesa e l'archivia/cestino.
func undoSenderModel(t *testing.T, mock *mail.Mock, sender Sender) Model {
	t.Helper()
	th, err := theme.Load("default")
	if err != nil {
		t.Fatalf("tema default: %v", err)
	}
	m := New(mock, sender, th)
	m, _ = step(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	return applyAll(t, m, loadRefs(mock, "", 0))
}

// Piano 10, Task 6 (D10-U2, ordine 1): l'invio è in attesa; poi archivia. `u` annulla
// l'archivia (non l'invio), e l'invio parte comunque allo scadere.
func TestUndoPrecedenceSendThenArchive(t *testing.T) {
	mock := undoFixture(t, false)
	rec := &senderRecorder{}
	m := undoSenderModel(t, mock, rec)
	m = confirmDrained(t, sendRequested(t, writing(m)))
	if m.undo == nil {
		t.Fatal("precondizione: nessun invio in attesa")
	}
	sendSeq := m.undo.seq
	m, cmd := step(m, pressKey("a"))
	m = applyAll(t, m, cmd)
	if m.lastUndo == nil {
		t.Fatal("precondizione: nessun annulla dopo archivia")
	}
	m, cmd = step(m, pressKey("u"))
	m = applyAll(t, m, cmd)
	if !hasInbox(mock, 1) {
		t.Error("u: l'archivia non è stato annullato")
	}
	if m.undo == nil || len(rec.calls) != 0 {
		t.Fatalf("u ha toccato l'invio: in attesa=%v, invii=%d", m.undo != nil, len(rec.calls))
	}
	m, cmd = step(m, undoDueMsg{seq: sendSeq})
	applyAll(t, m, cmd)
	if len(rec.calls) != 1 {
		t.Errorf("allo scadere l'invio è partito %d volte, voglio 1", len(rec.calls))
	}
}

// Piano 10, Task 6 (D10-U2, ordine 2): archivia, poi invio. L'invio sostituisce
// l'annulla dell'archivia: `u` annulla l'invio (riapre il composer), non l'archivia.
func TestUndoPrecedenceArchiveThenSend(t *testing.T) {
	mock := undoFixture(t, false)
	rec := &senderRecorder{}
	m := undoSenderModel(t, mock, rec)
	m, cmd := step(m, pressKey("a"))
	m = applyAll(t, m, cmd)
	if m.lastUndo == nil {
		t.Fatal("precondizione: nessun annulla dopo archivia")
	}
	m = confirmDrained(t, sendRequested(t, writing(m)))
	if m.lastUndo != nil {
		t.Fatal("l'invio non ha sostituito l'annulla dell'archivia")
	}
	m, _ = step(m, pressKey("u"))
	if m.currentView() != viewComposer {
		t.Errorf("u: vista = %v, voglio il composer dell'invio annullato", m.currentView())
	}
	if hasInbox(mock, 1) {
		t.Error("u ha annullato l'archivia invece dell'invio")
	}
}
