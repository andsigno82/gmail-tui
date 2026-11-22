package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/inbox"
)

// inTrash conta i messaggi del Mock nella mailbox del cestino.
func inTrash(mock *mail.Mock) int {
	n := 0
	for _, mm := range mock.Msgs {
		if box, _, err := mm.Ref.ID.Split(); err == nil && box == "[Gmail]/Trash" {
			n++
		}
	}
	return n
}

// hasInbox dice se il messaggio con quell'UID è ancora in INBOX.
func hasInbox(mock *mail.Mock, uid uint32) bool {
	for _, mm := range mock.Msgs {
		if mm.Ref.ID == msg.NewID("INBOX", uid) {
			return true
		}
	}
	return false
}

// openPagedThread porta la vista nel thread del primo messaggio della lista
// (fetch vero sul Mock, come farebbe il runtime).
func openPagedThread(t *testing.T, m Model) Model {
	t.Helper()
	m, cmd := step(m, inbox.SelectedMsg{Ref: m.refs[0]})
	for _, ms := range runCmd(cmd) {
		m, _ = step(m, ms)
	}
	if m.view != viewThread || !m.thread.Loaded() {
		t.Fatal("setup: thread non aperto")
	}
	return m
}

// --- apertura conferma (D4) -----------------------------------------------------

// TestTrashKeyAsksConfirmation: # apre la conferma sul messaggio sotto il
// cursore; nessuna op ancora, la barra dice cosa succede.
func TestTrashKeyAsksConfirmation(t *testing.T) {
	mock := &mail.Mock{Msgs: pagedMsgs(t, 3)}
	m := pagedModel(t, mock)
	m, cmd := step(m, pressKey("#"))
	if !m.trashOpen || cmd != nil {
		t.Fatalf("# : open=%v cmd=%v, voglio conferma aperta senza op", m.trashOpen, cmd != nil)
	}
	if m.trashRef.ID != msg.NewID("INBOX", 1) {
		t.Errorf("messaggio in conferma = %s, voglio quello sotto il cursore", m.trashRef.ID)
	}
	if bar := m.statusBar(); !strings.Contains(bar, "spostare nel cestino? s conferma · esc annulla") {
		t.Errorf("barra di conferma = %q", bar)
	}
}

// TestTrashConfirmMovesMessage: s sposta il messaggio nel cestino (il Mock
// riceve l'ID giusto), il messaggio esce da INBOX e la lista si rilegge.
func TestTrashConfirmMovesMessage(t *testing.T) {
	mock := &mail.Mock{Msgs: pagedMsgs(t, 3)}
	m := pagedModel(t, mock)
	m, _ = step(m, pressKey("j")) // secondo messaggio: UID 2
	m, _ = step(m, pressKey("#"))
	m, cmd := step(m, pressKey("s"))
	if m.trashOpen {
		t.Error("s non chiude la conferma")
	}
	m = settle(t, m, cmd, 4)
	if hasInbox(mock, 2) || inTrash(mock) != 1 {
		t.Errorf("dopo s: UID 2 in INBOX=%v, messaggi nel cestino=%d; voglio UID 2 spostato", hasInbox(mock, 2), inTrash(mock))
	}
	if !strings.Contains(m.statusText(), "spostata nel cestino") || m.status.isErr {
		t.Errorf("status = %q, voglio il feedback di successo", m.statusText())
	}
	if len(m.refs) != 2 {
		t.Errorf("refs dopo il refresh = %d, voglio 2", len(m.refs))
	}
}

// TestTrashEscCancels: esc annulla la conferma senza toccare lo store.
func TestTrashEscCancels(t *testing.T) {
	mock := &mail.Mock{Msgs: pagedMsgs(t, 3)}
	m := pagedModel(t, mock)
	m, _ = step(m, pressKey("#"))
	m, cmd := step(m, pressKey("esc"))
	if m.trashOpen || cmd != nil {
		t.Errorf("esc: open=%v cmd=%v, voglio chiusa senza op", m.trashOpen, cmd != nil)
	}
	if !hasInbox(mock, 1) || inTrash(mock) != 0 {
		t.Error("esc ha spostato il messaggio")
	}
}

// TestTrashConfirmIsModal: con la conferma aperta le altre op (a, s) non
// scattano e il mouse è ignorato.
func TestTrashConfirmIsModal(t *testing.T) {
	mock := &mail.Mock{Msgs: pagedMsgs(t, 3)}
	m := pagedModel(t, mock)
	m, _ = step(m, pressKey("#"))
	m, cmd := step(m, pressKey("a"))
	if cmd != nil || !m.trashOpen {
		t.Errorf("a con la conferma aperta: cmd=%v open=%v", cmd != nil, m.trashOpen)
	}
	m, cmd = step(m, tea.MouseClickMsg{Button: tea.MouseLeft, Y: 1})
	if cmd != nil || m.inbox.SelectedIndex() != 0 {
		t.Error("click con la conferma aperta ha agito sulla lista")
	}
	if !hasInbox(mock, 1) {
		t.Error("un tasto dietro la conferma ha toccato il messaggio")
	}
}

// --- errori e stati -----------------------------------------------------------

// TestTrashErrorShowsStatusAndKeepsList: cestino assente → errore in status,
// INBOX intatta, lista invariata.
func TestTrashErrorShowsStatusAndKeepsList(t *testing.T) {
	mock := &mail.Mock{
		Msgs:      pagedMsgs(t, 3),
		Mailboxes: []mail.Mailbox{{Name: "INBOX", Role: "inbox"}},
	}
	m := pagedModel(t, mock)
	m, _ = step(m, pressKey("#"))
	m, cmd := step(m, pressKey("s"))
	m = settle(t, m, cmd, 4)
	if !m.status.isErr || !strings.Contains(m.statusText(), "cestino non trovato") {
		t.Errorf("status = %q (err=%v), voglio errore cestino non trovato", m.statusText(), m.status.isErr)
	}
	if !hasInbox(mock, 1) || len(m.refs) != 3 {
		t.Errorf("errore: UID 1 in INBOX=%v, refs=%d; voglio intatto", hasInbox(mock, 1), len(m.refs))
	}
}

// TestTrashInTrashFolderIsRefused: dalla vista Cestino # non apre la conferma.
func TestTrashInTrashFolderIsRefused(t *testing.T) {
	mock := folderMock(t)
	rec := &recorder{MailStore: mock}
	m := folderModel(t, rec)
	m, cmd := pickFolder(t, openPicker(t, m), 2) // Cestino
	m = applyAll(t, m, cmd)
	m, cmd = step(m, pressKey("#"))
	if m.trashOpen || cmd != nil {
		t.Error("# nel cestino ha aperto la conferma")
	}
	if m.statusText() != "già nel cestino" {
		t.Errorf("status = %q, voglio \"già nel cestino\"", m.statusText())
	}
}

// TestTrashWithoutMessageIsNoop: lista vuota → # non fa nulla.
func TestTrashWithoutMessageIsNoop(t *testing.T) {
	mock := &mail.Mock{}
	m := pagedModel(t, mock)
	m, _ = step(m, pressKey("#"))
	if m.trashOpen {
		t.Error("# con lista vuota ha aperto la conferma")
	}
}

// --- dalla thread view (D7) ----------------------------------------------------

// TestTrashFromThreadReturnsToList: # nel thread → conferma; s sposta e torna
// alla lista (il messaggio non è più nella cartella).
func TestTrashFromThreadReturnsToList(t *testing.T) {
	mock := &mail.Mock{Msgs: pagedMsgs(t, 3)}
	m := openPagedThread(t, pagedModel(t, mock))
	m, _ = step(m, pressKey("#"))
	if !m.trashOpen || m.view != viewThread {
		t.Fatalf("# nel thread: open=%v view=%v, voglio conferma sul thread", m.trashOpen, m.view)
	}
	m, cmd := step(m, pressKey("s"))
	if m.view != viewInbox {
		t.Errorf("dopo s dal thread: view = %v, voglio la lista", m.view)
	}
	m = settle(t, m, cmd, 4)
	if hasInbox(mock, 1) || inTrash(mock) != 1 {
		t.Error("dal thread il messaggio non è stato spostato")
	}
}

// TestTrashCancelFromThreadStaysInThread: esc dalla conferma nel thread lascia
// il thread aperto.
func TestTrashCancelFromThreadStaysInThread(t *testing.T) {
	mock := &mail.Mock{Msgs: pagedMsgs(t, 3)}
	m := openPagedThread(t, pagedModel(t, mock))
	m, _ = step(m, pressKey("#"))
	m, _ = step(m, pressKey("esc"))
	if m.view != viewThread || m.trashOpen {
		t.Errorf("esc dal thread: view=%v open=%v, voglio thread e conferma chiusa", m.view, m.trashOpen)
	}
}

// TestTrashRowFromInboxInTrashFolderOpensConfirmation (Piano 7.1, Task 5): in
// vista Cestino una ricerca label:INBOX porta righe di INBOX. Il ruolo che conta
// è quello della mailbox della riga: # apre la conferma, non dichiara il
// messaggio già nel cestino.
func TestTrashRowFromInboxInTrashFolderOpensConfirmation(t *testing.T) {
	m := folderModel(t, &recorder{MailStore: folderMock(t)})
	m.folder = mail.Mailbox{Name: "[Gmail]/Trash", Role: "trash"}
	m.refs = []msg.MessageRef{{ID: msg.NewID("INBOX", 3), Subject: "posta in arrivo"}}
	m.inbox.SetRefs(m.refs)

	m, cmd := step(m, pressKey("#"))
	if m.statusText() == "già nel cestino" {
		t.Fatal("riga di INBOX dalla vista Cestino: dichiarata già nel cestino")
	}
	if !m.trashOpen || cmd != nil {
		t.Fatalf("# : open=%v cmd=%v, voglio conferma aperta", m.trashOpen, cmd != nil)
	}
	if m.trashRef.ID != msg.NewID("INBOX", 3) {
		t.Errorf("messaggio in conferma = %s, voglio la riga di INBOX", m.trashRef.ID)
	}
}

// TestTrashRowFromTrashInOtherFolderOpensConfirmation (Piano 7.1, Task 5): una
// riga del cestino vista da un'altra cartella ha ruolo ignoto all'app: la
// conferma si apre e decide il backend (che rifiuta gli id già nel cestino).
func TestTrashRowFromTrashInOtherFolderOpensConfirmation(t *testing.T) {
	m := folderModel(t, &recorder{MailStore: folderMock(t)})
	m.folder = mail.Mailbox{Name: "[Gmail]/Sent", Role: "sent"}
	m.refs = []msg.MessageRef{{ID: msg.NewID("[Gmail]/Trash", 7), Subject: "nel cestino"}}
	m.inbox.SetRefs(m.refs)

	m, cmd := step(m, pressKey("#"))
	if !m.trashOpen || cmd != nil || m.statusText() == "già nel cestino" {
		t.Fatalf("# su riga del cestino da Inviati: open=%v cmd=%v status=%q, voglio conferma", m.trashOpen, cmd != nil, m.statusText())
	}
}
