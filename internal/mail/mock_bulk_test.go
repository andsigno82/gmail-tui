package mail

// Piano 10, Task 4: contratto e Mock per annulla e label su selezione (D10-T2..T5).
// Il Mock simula Gmail (come ListConversations); NoGmail = true simula un server
// senza X-GM-EXT-1.

import (
	"context"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// mockSmistamento è il Mock di Piano 10: INBOX con 5 messaggi (newMock), cestino,
// Tutti i messaggi e una label "Lavoro". Ogni messaggio INBOX ha un GmailID e una
// copia in Tutti i messaggi con lo stesso GmailID (come Gmail).
func mockSmistamento(t *testing.T) *Mock {
	t.Helper()
	m := newMock()
	m.Mailboxes = []Mailbox{
		{Name: "INBOX", Role: "inbox"},
		{Name: "[Gmail]/Trash", Role: "trash"},
		{Name: "[Gmail]/All Mail", Role: "all"},
		{Name: "Lavoro", Role: ""},
	}
	for _, mm := range append([]*msg.Message(nil), m.Msgs...) {
		uid := uint32(0)
		if _, u, err := mm.Ref.ID.Split(); err == nil {
			uid = u
		}
		mm.Ref.GmailID = 1000 + uint64(uid)
		cp := *mm
		cp.Ref.ID = msg.NewID("[Gmail]/All Mail", uid)
		m.Msgs = append(m.Msgs, &cp)
	}
	return m
}

// TestMockTrashReturnsIDsInTrash: Trash restituisce l'ID nel cestino di ogni
// messaggio spostato (serve a Restore, D10-T2).
func TestMockTrashReturnsIDsInTrash(t *testing.T) {
	m := newMock()
	got, err := m.Trash(context.Background(), []msg.MessageID{msg.NewID("INBOX", 2), msg.NewID("INBOX", 4)})
	if err != nil {
		t.Fatalf("Trash: %v", err)
	}
	want := []msg.MessageID{msg.NewID("[Gmail]/Trash", 1), msg.NewID("[Gmail]/Trash", 2)}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("Trash restituisce %v, voglio %v", got, want)
	}
}

// TestMockRestoreMovesBackToOrigin: Restore riporta i messaggi dal cestino alla
// cartella indicata con un nuovo UID e restituisce i nuovi ID.
func TestMockRestoreMovesBackToOrigin(t *testing.T) {
	m := newMock()
	ctx := context.Background()
	trashed, err := m.Trash(ctx, []msg.MessageID{msg.NewID("INBOX", 2)})
	if err != nil {
		t.Fatalf("Trash: %v", err)
	}
	restored, err := m.Restore(ctx, trashed, "INBOX")
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if len(restored) != 1 || restored[0] != msg.NewID("INBOX", 6) {
		t.Fatalf("Restore restituisce %v, voglio [INBOX:6]", restored)
	}
	inbox, _ := m.ListMessages(ctx, msg.Query{}, Page{})
	if len(inbox) != 5 {
		t.Errorf("INBOX dopo Restore = %d, voglio 5", len(inbox))
	}
	if trash, _ := m.ListMessages(ctx, msg.Query{Label: "[Gmail]/Trash"}, Page{}); len(trash) != 0 {
		t.Errorf("cestino dopo Restore = %d, voglio 0", len(trash))
	}
}

// TestMockRestoreRefusesIDsOutsideTrash: un ID fuori dal cestino o una
// destinazione inesistente rifiutano senza alcuna azione.
func TestMockRestoreRefusesIDsOutsideTrash(t *testing.T) {
	m := newMock()
	ctx := context.Background()
	if _, err := m.Restore(ctx, []msg.MessageID{msg.NewID("INBOX", 1)}, "INBOX"); err == nil {
		t.Error("Restore di un ID in INBOX: err = nil, voglio rifiuto")
	}
	if _, err := m.Restore(ctx, []msg.MessageID{msg.NewID("INBOX", 1)}, "Inesistente"); err == nil {
		t.Error("Restore verso cartella inesistente: err = nil, voglio rifiuto")
	}
	if got, _ := m.ListMessages(ctx, msg.Query{}, Page{}); len(got) != 5 {
		t.Errorf("INBOX dopo rifiuti = %d, voglio 5 (nessuna azione)", len(got))
	}
}

// TestMockAllMailIDsRefusesWithoutGmail: senza X-GM-EXT-1 errore; con Gmail
// restituisce l'ID della copia in Tutti i messaggi, solo per gli ID in INBOX.
func TestMockAllMailIDsRefusesWithoutGmail(t *testing.T) {
	ctx := context.Background()
	m := mockSmistamento(t)
	m.NoGmail = true
	if _, err := m.AllMailIDs(ctx, []msg.MessageID{msg.NewID("INBOX", 1)}); err == nil {
		t.Error("AllMailIDs senza Gmail: err = nil, voglio errore")
	}

	m.NoGmail = false
	got, err := m.AllMailIDs(ctx, []msg.MessageID{msg.NewID("INBOX", 1), msg.NewID("INBOX", 3)})
	if err != nil {
		t.Fatalf("AllMailIDs con Gmail: %v", err)
	}
	if len(got) != 2 || got[0] != msg.NewID("[Gmail]/All Mail", 1) || got[1] != msg.NewID("[Gmail]/All Mail", 3) {
		t.Errorf("AllMailIDs = %v, voglio gli ID in Tutti i messaggi di UID 1 e 3", got)
	}
	if _, err := m.AllMailIDs(ctx, []msg.MessageID{msg.NewID("[Gmail]/All Mail", 1)}); err == nil {
		t.Error("AllMailIDs su un ID non in INBOX: err = nil, voglio errore")
	}
}

// TestMockModifyLabelsAddRemove: con Gmail add e remove insieme cambiano le label
// di tutte le copie del messaggio (INBOX e Tutti i messaggi), senza duplicare.
func TestMockModifyLabelsAddRemove(t *testing.T) {
	ctx := context.Background()
	m := mockSmistamento(t)
	m.Msgs[0].Ref.Labels = []string{"Vecchia"}
	m.Msgs[5].Ref.Labels = []string{"Vecchia"} // copia in Tutti i messaggi
	if err := m.ModifyLabels(ctx, []msg.MessageID{msg.NewID("INBOX", 1), msg.NewID("INBOX", 3)}, []string{"Lavoro"}, []string{"Vecchia"}); err != nil {
		t.Fatalf("ModifyLabels: %v", err)
	}
	for _, mm := range m.Msgs {
		if mm.Ref.GmailID == 1001 && !hasLabelsExactly(mm.Ref.Labels, "Lavoro") {
			t.Errorf("copia di UID 1 in %s: label = %v, voglio [Lavoro]", mm.Ref.ID, mm.Ref.Labels)
		}
	}
	if inbox, _ := m.ListMessages(ctx, msg.Query{}, Page{}); len(inbox) != 5 {
		t.Errorf("INBOX = %d messaggi, voglio 5 (nessuna copia creata)", len(inbox))
	}
	if mm := m.find(msg.NewID("INBOX", 2)); mm == nil || len(mm.Ref.Labels) != 0 {
		t.Errorf("messaggio non selezionato toccato: %+v", mm)
	}
}

// TestMockModifyLabelsInboxIsArchive: \Inbox rimosso toglie il messaggio da INBOX
// (archivia); \Inbox aggiunto sulla copia in Tutti i messaggi lo riporta in INBOX.
func TestMockModifyLabelsInboxIsArchive(t *testing.T) {
	ctx := context.Background()
	m := mockSmistamento(t)
	if err := m.ModifyLabels(ctx, []msg.MessageID{msg.NewID("INBOX", 1)}, nil, []string{`\Inbox`}); err != nil {
		t.Fatalf("ModifyLabels remove \\Inbox: %v", err)
	}
	if inbox, _ := m.ListMessages(ctx, msg.Query{}, Page{}); len(inbox) != 4 {
		t.Fatalf("INBOX dopo rimozione di \\Inbox = %d, voglio 4", len(inbox))
	}
	allMail, _ := m.ListMessages(ctx, msg.Query{Label: "[Gmail]/All Mail"}, Page{})
	if len(allMail) != 5 {
		t.Errorf("Tutti i messaggi = %d, voglio 5 (il messaggio resta in All Mail)", len(allMail))
	}
	if err := m.ModifyLabels(ctx, []msg.MessageID{msg.NewID("[Gmail]/All Mail", 1)}, []string{`\Inbox`}, nil); err != nil {
		t.Fatalf("ModifyLabels add \\Inbox: %v", err)
	}
	if inbox, _ := m.ListMessages(ctx, msg.Query{}, Page{}); len(inbox) != 5 {
		t.Errorf("INBOX dopo add di \\Inbox = %d, voglio 5", len(inbox))
	}
}

// TestMockModifyLabelsRejectsSystemLabels: gli attributi di sistema (con backslash)
// diversi da \Inbox non sono label utente e sono rifiutati senza azioni.
func TestMockModifyLabelsRejectsSystemLabels(t *testing.T) {
	ctx := context.Background()
	m := mockSmistamento(t)
	if err := m.ModifyLabels(ctx, []msg.MessageID{msg.NewID("INBOX", 1)}, []string{`\Starred`}, nil); err == nil {
		t.Error("ModifyLabels con \\Starred: err = nil, voglio rifiuto")
	}
	if mm := m.find(msg.NewID("INBOX", 1)); mm == nil || len(mm.Ref.Labels) != 0 {
		t.Errorf("messaggio toccato nonostante il rifiuto: %+v", mm)
	}
}

// TestMockModifyLabelsRemoveOutsideLabelRefused: senza Gmail la rimozione di una
// label richiede che ogni ID stia in quella label (altrimenti rifiuto prima di
// toccare il Mock); l'add copia nella label.
func TestMockModifyLabelsRemoveOutsideLabelRefused(t *testing.T) {
	ctx := context.Background()
	m := mockSmistamento(t)
	m.NoGmail = true
	if err := m.ModifyLabels(ctx, []msg.MessageID{msg.NewID("INBOX", 1)}, nil, []string{"Lavoro"}); err == nil {
		t.Fatal("rimozione di Lavoro da un ID in INBOX: err = nil, voglio rifiuto")
	}
	if got, _ := m.ListMessages(ctx, msg.Query{}, Page{}); len(got) != 5 {
		t.Errorf("INBOX dopo rifiuto = %d, voglio 5", len(got))
	}

	if err := m.ModifyLabels(ctx, []msg.MessageID{msg.NewID("INBOX", 1)}, []string{"Lavoro"}, nil); err != nil {
		t.Fatalf("add a Lavoro: %v", err)
	}
	if got, _ := m.ListMessages(ctx, msg.Query{Label: "Lavoro"}, Page{}); len(got) != 1 {
		t.Fatalf("Lavoro dopo add = %d, voglio 1", len(got))
	}
	if err := m.ModifyLabels(ctx, []msg.MessageID{msg.NewID("Lavoro", 1)}, nil, []string{"Lavoro"}); err != nil {
		t.Fatalf("remove da Lavoro: %v", err)
	}
	if got, _ := m.ListMessages(ctx, msg.Query{Label: "Lavoro"}, Page{}); len(got) != 0 {
		t.Errorf("Lavoro dopo remove = %d, voglio 0", len(got))
	}
	if err := m.ModifyLabels(ctx, []msg.MessageID{msg.NewID("INBOX", 1)}, []string{`\Inbox`}, nil); err == nil {
		t.Error("senza Gmail \\Inbox come label: err = nil, voglio rifiuto")
	}
}

// hasLabelsExactly dice se labels è esattamente [want].
func hasLabelsExactly(labels []string, want string) bool {
	return len(labels) == 1 && labels[0] == want
}
