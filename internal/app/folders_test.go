package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// folderMock è lo store dei test di cartella: INBOX con 60 messaggi, una
// cartella Inviati con 2 messaggi, il cestino e una label "Work".
func folderMock(tb testing.TB) *mail.Mock {
	tb.Helper()
	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	sent := []*msg.Message{
		{Ref: msg.MessageRef{ID: msg.NewID("[Gmail]/Sent", 1), From: "me@x.dev", Subject: "inviata uno", Date: base}},
		{Ref: msg.MessageRef{ID: msg.NewID("[Gmail]/Sent", 2), From: "me@x.dev", Subject: "inviata due", Date: base.Add(-time.Hour)}},
	}
	return &mail.Mock{
		Msgs: append(pagedMsgs(tb, 60), sent...),
		Mailboxes: []mail.Mailbox{
			{Name: "INBOX", Role: "inbox"},
			{Name: "[Gmail]/Sent", Role: "sent"},
			{Name: "[Gmail]/Trash", Role: "trash"},
			{Name: "Work"},
		},
	}
}

// folderModel è il Model radice sullo store di folderMock, con il load della
// posta in arrivo consolidato (cursore in cima, nessuna op in volo).
func folderModel(tb testing.TB, rec *recorder) Model {
	tb.Helper()
	return pagedModel(tb, rec)
}

// openPicker preme m e applica la risposta di ListMailboxes.
func openPicker(tb testing.TB, m Model) Model {
	tb.Helper()
	m, cmd := step(m, pressKey("m"))
	if !m.folderOpen {
		tb.Fatal("m non apre il selettore cartelle")
	}
	return applyAll(tb, m, cmd)
}

// pickFolder sceglie nel selettore la cartella di indice i (cursore + invio).
func pickFolder(tb testing.TB, m Model, i int) (Model, tea.Cmd) {
	tb.Helper()
	m.folderList.Select(i)
	return step(m, pressKey("enter"))
}

// --- apertura e chiusura ----------------------------------------------------

// TestFolderPickerOpensAndLists: m apre la modale, ListMailboxes riempie la
// lista nell'ordine del backend e il cursore parte dalla cartella corrente.
func TestFolderPickerOpensAndLists(t *testing.T) {
	rec := &recorder{MailStore: folderMock(t)}
	m := openPicker(t, folderModel(t, rec))
	if !m.folderOpen || m.pending != 0 {
		t.Fatalf("dopo il caricamento: open=%v pending=%d, voglio true e 0", m.folderOpen, m.pending)
	}
	if got := len(m.folderList.Items()); got != 4 {
		t.Fatalf("cartelle nel selettore = %d, voglio 4", got)
	}
	if v := m.View().Content; !strings.Contains(v, "Cestino") || !strings.Contains(v, "Inviati") {
		t.Errorf("selettore senza nomi leggibili: %q", v)
	}
}

// TestFolderPickerEscCloses: esc chiude la modale senza cambiare cartella;
// una risposta di ListMailboxes arrivata dopo la chiusura è ignorata.
func TestFolderPickerEscCloses(t *testing.T) {
	rec := &recorder{MailStore: folderMock(t)}
	m := folderModel(t, rec)
	m, cmd := step(m, pressKey("m"))
	m, _ = step(m, pressKey("esc"))
	if m.folderOpen {
		t.Fatal("esc non chiude il selettore")
	}
	m = applyAll(t, m, cmd) // risposta tardiva
	if m.folderOpen || len(m.folderList.Items()) != 0 || m.folder.Name != "" {
		t.Errorf("risposta tardiva dopo esc: open=%v items=%d folder=%q", m.folderOpen, len(m.folderList.Items()), m.folder.Name)
	}
	if m.pending != 0 {
		t.Errorf("pending = %d dopo la risposta tardiva, voglio 0", m.pending)
	}
}

// TestFolderPickerListError: errore di ListMailboxes → status d'errore, nessuna modale.
func TestFolderPickerListError(t *testing.T) {
	rec := &recorder{MailStore: folderMock(t)}
	m := folderModel(t, rec)
	m, _ = step(m, pressKey("m"))
	m, _ = step(m, mailboxesLoaded{err: errors.New("boom")})
	if m.folderOpen || !m.status.isErr || !strings.Contains(m.statusText(), "boom") {
		t.Errorf("errore cartelle: open=%v status=%q", m.folderOpen, m.statusText())
	}
}

// --- scelta cartella ----------------------------------------------------------

// TestChooseFolderListsItsMessages: invio sulla cartella scelta rilista con
// Query.Label = nome IMAP, resetta lo scroll e porta il cursore in cima.
func TestChooseFolderListsItsMessages(t *testing.T) {
	rec := &recorder{MailStore: folderMock(t)}
	m := openPicker(t, folderModel(t, rec))
	m, cmd := pickFolder(t, m, 1) // Inviati
	if m.folderOpen {
		t.Fatal("invio non chiude il selettore")
	}
	m = applyAll(t, m, cmd)
	last := rec.lists[len(rec.lists)-1]
	if last.Label != "[Gmail]/Sent" {
		t.Errorf("Query.Label = %q, voglio [Gmail]/Sent", last.Label)
	}
	if len(m.refs) != 2 || m.refs[0].Subject != "inviata uno" {
		t.Errorf("refs = %d (%v), voglio i 2 messaggi inviati", len(m.refs), m.refs)
	}
	if m.folder.Name != "[Gmail]/Sent" {
		t.Errorf("cartella corrente = %q", m.folder.Name)
	}
	if m.exhausted != true || m.loadingMore {
		t.Errorf("paginazione non resettata: exhausted=%v loadingMore=%v", m.exhausted, m.loadingMore)
	}
	if got := m.inbox.SelectedIndex(); got != 0 {
		t.Errorf("cursore dopo il cambio cartella = %d, voglio 0", got)
	}
	if !strings.Contains(m.statusText(), "Inviati") {
		t.Errorf("status = %q, voglio il nome della cartella", m.statusText())
	}
}

// TestChooseInboxReturnsToDefault: scegliere la posta in arrivo torna alla
// cartella di default (zero value, nessun tag in status bar).
func TestChooseInboxReturnsToDefault(t *testing.T) {
	rec := &recorder{MailStore: folderMock(t)}
	m := openPicker(t, folderModel(t, rec))
	m, cmd := pickFolder(t, m, 1)
	m = applyAll(t, m, cmd)
	m = openPicker(t, m)
	m, cmd = pickFolder(t, m, 0) // Posta in arrivo
	m = applyAll(t, m, cmd)
	if m.folder != (mail.Mailbox{}) {
		t.Errorf("cartella dopo INBOX = %+v, voglio zero value", m.folder)
	}
	if strings.Contains(m.statusBar(), "Inviati") {
		t.Error("tag della cartella ancora in status bar dopo il ritorno in INBOX")
	}
}

// TestChooseSameFolderOnlyCloses: scegliere la cartella già aperta chiude senza
// ricaricare.
func TestChooseSameFolderOnlyCloses(t *testing.T) {
	rec := &recorder{MailStore: folderMock(t)}
	m := openPicker(t, folderModel(t, rec))
	before := len(rec.lists)
	m, cmd := pickFolder(t, m, 0) // già in INBOX
	if m.folderOpen {
		t.Error("stessa cartella: il selettore resta aperto")
	}
	for _, ms := range runCmd(cmd) {
		if _, ok := ms.(msgsLoaded); ok {
			t.Error("stessa cartella: ricaricata la lista")
		}
	}
	if len(rec.lists) != before {
		t.Errorf("stessa cartella: liste %d→%d, voglio nessuna lettura", before, len(rec.lists))
	}
}

// TestFolderSwitchClearsSearch: cambio cartella con una ricerca attiva la
// azzera (D6): la cartella nuova si apre senza filtri.
func TestFolderSwitchClearsSearch(t *testing.T) {
	rec := &recorder{MailStore: folderMock(t)}
	m := folderModel(t, rec)
	m.queryRaw = "bob"
	m.query = msg.Query{From: "bob"}
	m = openPicker(t, m)
	m, cmd := pickFolder(t, m, 1)
	m = applyAll(t, m, cmd)
	if m.queryRaw != "" || m.query != (msg.Query{}) {
		t.Errorf("ricerca dopo il cambio cartella: raw=%q query=%+v, voglio vuota", m.queryRaw, m.query)
	}
	if got := rec.lists[len(rec.lists)-1]; got.Label != "[Gmail]/Sent" || got.From != "" {
		t.Errorf("lista dopo il cambio: %+v, voglio solo la cartella", got)
	}
}

// --- ricerca e refresh dentro la cartella -------------------------------------

// TestSearchInFolderInheritsLabel: una ricerca senza label: esplicita cerca
// nella cartella corrente; con label: esplicita vince la label digitata.
func TestSearchInFolderInheritsLabel(t *testing.T) {
	rec := &recorder{MailStore: folderMock(t)}
	m := folderModel(t, rec)
	m, cmd := pickFolder(t, openPicker(t, m), 1)
	m = applyAll(t, m, cmd)

	out := runCmd(submitQuery(rec, "bob", m.folder.Name))
	if len(out) != 1 {
		t.Fatalf("submitQuery: %d msg, voglio 1", len(out))
	}
	if got := rec.search[len(rec.search)-1].Label; got != "[Gmail]/Sent" {
		t.Errorf("ricerca senza label: Label = %q, voglio la cartella corrente", got)
	}
	runCmd(submitQuery(rec, "label:Work bob", m.folder.Name))
	if got := rec.search[len(rec.search)-1].Label; got != "Work" {
		t.Errorf("ricerca con label: Label = %q, voglio Work", got)
	}
}

// TestRefreshStaysInFolder: il refresh dopo un'op rilista la cartella corrente.
func TestRefreshStaysInFolder(t *testing.T) {
	rec := &recorder{MailStore: folderMock(t)}
	m := folderModel(t, rec)
	m, cmd := pickFolder(t, openPicker(t, m), 1)
	m = applyAll(t, m, cmd)
	m, cmd = step(m, pressKey("s")) // stella: refresh a prima pagina
	settle(t, m, cmd, 4)            // il refresh rilegge la lista: solo la query conta
	if got := rec.lists[len(rec.lists)-1].Label; got != "[Gmail]/Sent" {
		t.Errorf("refresh dopo op: Label = %q, voglio la cartella corrente", got)
	}
}

// TestStaleResultFromOtherFolderDiscarded: un risultato di un refresh partito
// nella cartella precedente non sovrascrive la lista della cartella nuova.
func TestStaleResultFromOtherFolderDiscarded(t *testing.T) {
	rec := &recorder{MailStore: folderMock(t)}
	m := folderModel(t, rec)
	m, cmd := pickFolder(t, openPicker(t, m), 1)
	m = applyAll(t, m, cmd)
	m, _ = step(m, msgsLoaded{refs: pagedRefs(t, 50), limit: 50, silent: true, mailbox: ""})
	if len(m.refs) != 2 {
		t.Errorf("risultato stantio applicato: refs=%d, voglio 2 (Inviati)", len(m.refs))
	}
}

// TestStaleMoreFromOtherFolderDiscarded: una pagina successiva della cartella
// precedente non si accoda alla cartella nuova.
func TestStaleMoreFromOtherFolderDiscarded(t *testing.T) {
	rec := &recorder{MailStore: folderMock(t)}
	m := folderModel(t, rec)
	m, _ = step(m, pressKey("G")) // pagina successiva di INBOX in volo
	m = openPicker(t, m)
	m, cmd := pickFolder(t, m, 1)
	m = applyAll(t, m, cmd)
	m, _ = step(m, moreLoaded{refs: pagedRefs(t, 50), query: "", mailbox: ""})
	if len(m.refs) != 2 {
		t.Errorf("pagina della cartella precedente accodata: refs=%d, voglio 2", len(m.refs))
	}
}

// TestFolderSwitchResetsLoadingMore: cambiare cartella con una pagina in volo
// libera lo scroll della cartella nuova.
func TestFolderSwitchResetsLoadingMore(t *testing.T) {
	rec := &recorder{MailStore: folderMock(t)}
	m := folderModel(t, rec)
	m, _ = step(m, pressKey("G"))
	if !m.loadingMore {
		t.Fatal("setup: pagina in volo attesa")
	}
	m = openPicker(t, m)
	m, cmd := pickFolder(t, m, 1)
	if m.loadingMore {
		t.Error("loadingMore ancora acceso dopo il cambio cartella")
	}
	_ = applyAll(t, m, cmd)
}

// --- gate: tasti e mouse con la modale aperta ----------------------------------

// TestFolderPickerBlocksGlobalKeysAndMouse: con la modale aperta le op globali
// (a, s) e il mouse non agiscono sulla lista dietro.
func TestFolderPickerBlocksGlobalKeysAndMouse(t *testing.T) {
	rec := &recorder{MailStore: folderMock(t)}
	m := openPicker(t, folderModel(t, rec))
	m, cmd := step(m, pressKey("a"))
	if cmd != nil || !m.folderOpen {
		t.Errorf("a con la modale aperta: cmd=%v open=%v", cmd != nil, m.folderOpen)
	}
	m, cmd = step(m, pressKey("s"))
	if cmd != nil {
		t.Error("s con la modale aperta ha prodotto una op")
	}
	m, cmd = step(m, tea.MouseClickMsg{Button: tea.MouseLeft, Y: 0})
	if cmd != nil || m.inbox.SelectedIndex() != 0 {
		t.Errorf("click con la modale aperta: cmd=%v cursore=%d", cmd != nil, m.inbox.SelectedIndex())
	}
	m, _ = step(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if m.inbox.SelectedIndex() != 0 {
		t.Error("rotella con la modale aperta ha mosso il cursore dietro")
	}
}

// --- status bar ----------------------------------------------------------------

// TestStatusBarShowsFolderTag: fuori dalla posta in arrivo la status bar dice
// in quale cartella si è.
func TestStatusBarShowsFolderTag(t *testing.T) {
	rec := &recorder{MailStore: folderMock(t)}
	m := folderModel(t, rec)
	m, cmd := pickFolder(t, openPicker(t, m), 2) // Cestino
	m = applyAll(t, m, cmd)
	if !strings.Contains(m.statusBar(), "[Cestino]") {
		t.Errorf("status bar senza il tag della cartella: %q", m.statusBar())
	}
}

// TestFolderNames: etichette leggibili per i ruoli, nome IMAP per le label.
func TestFolderNames(t *testing.T) {
	cases := map[string]mail.Mailbox{
		"Posta in arrivo": {Name: "INBOX", Role: "inbox"},
		"Inviati":         {Name: "[Gmail]/Sent", Role: "sent"},
		"Cestino":         {Name: "[Gmail]/Trash", Role: "trash"},
		"Bozze":           {Name: "[Gmail]/Drafts", Role: "drafts"},
		"Spam":            {Name: "[Gmail]/Spam", Role: "junk"},
		"Work":            {Name: "Work"},
	}
	for want, box := range cases {
		if got := folderName(box); got != want {
			t.Errorf("folderName(%+v) = %q, voglio %q", box, got, want)
		}
	}
}

// --- tasti documentati ---------------------------------------------------------

// TestFolderShortHelpWhilePicking: con la modale aperta la barra mostra come
// usarla (invio e annulla).
func TestFolderShortHelpWhilePicking(t *testing.T) {
	rec := &recorder{MailStore: folderMock(t)}
	m := openPicker(t, folderModel(t, rec))
	descs := bindingDescs(m.shortHelpFor(false, viewInbox))
	if !descs["annulla"] {
		t.Errorf("help del selettore senza annulla: %v", descs)
	}
}
