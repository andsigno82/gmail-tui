package app

import (
	"context"
	"errors"
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

// Piano 10, review punto 1: un archivia sostituisce l'annulla dell'invio in attesa.
// Il toast dell'invio non deve più offrire `u` (che ora annulla l'archivia): diventa
// "invio tra N s" e l'invio parte comunque allo scadere.
func TestSendToastDropsUndoWhenArchiveReplacesIt(t *testing.T) {
	mock := undoFixture(t, false)
	rec := &senderRecorder{}
	m := undoSenderModel(t, mock, rec)
	m = confirmDrained(t, sendRequested(t, writing(m)))
	if !toastContains(m, "invio tra 5 s — u annulla") {
		t.Fatalf("precondizione: toast = %v, voglio l'invio con u", m.toast.Texts())
	}
	m, cmd := step(m, pressKey("a"))
	m = applyAll(t, m, cmd)
	for _, s := range m.toast.Texts() {
		if strings.Contains(s, "invio tra") && strings.Contains(s, "u annulla") {
			t.Errorf("toast dell'invio = %q: dopo l'archivia `u` non annulla più l'invio", s)
		}
	}
	if !toastContains(m, "invio tra 5 s") {
		t.Errorf("toast = %v, voglio \"invio tra 5 s\" senza u", m.toast.Texts())
	}
}

// toastContains dice se un toast contiene il testo dato.
func toastContains(m Model, text string) bool {
	for _, s := range m.toast.Texts() {
		if strings.Contains(s, text) {
			return true
		}
	}
	return false
}

// failSecondTrash fa fallire la seconda Trash: un cestino multiplo con errore a metà
// (Piano 10, review punto 2).
type failSecondTrash struct {
	*mail.Mock
	calls int
}

// Trash delega al Mock, tranne la seconda chiamata che fallisce senza agire.
func (f *failSecondTrash) Trash(ctx context.Context, ids []msg.MessageID) ([]msg.MessageID, error) {
	f.calls++
	if f.calls == 2 {
		return nil, errors.New("server giù")
	}
	return f.Mock.Trash(ctx, ids)
}

// inboxTrioFixture: tre messaggi in INBOX (uid 1..3, GmailID 11..13), senza thread:
// ogni riga è una conversazione, come con le fixture di archivia.
func inboxTrioFixture() *mail.Mock {
	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	var msgs []*msg.Message
	for uid := uint32(1); uid <= 3; uid++ {
		msgs = append(msgs, &msg.Message{Ref: msg.MessageRef{
			ID: msg.NewID("INBOX", uid), GmailID: uint64(10 + uid), From: "anna@x.dev",
			Subject: "tema", Date: base.Add(time.Duration(uid) * time.Hour),
		}})
	}
	return &mail.Mock{
		Mailboxes: []mail.Mailbox{
			{Name: "INBOX", Role: "inbox"},
			{Name: "[Gmail]/Tutti i messaggi", Role: "all"},
			{Name: "[Gmail]/Trash", Role: "trash"},
		},
		Msgs: msgs,
	}
}

// Piano 10, review punto 2: cestino di tre conversazioni con errore alla seconda. La
// prima è spostata: il toast offre `u` per lei (ritorno al suo posto); la seconda e la
// terza restano in INBOX e il toast d'errore le dice.
func TestBulkTrashPartialErrorOffersUndoForMoved(t *testing.T) {
	mock := inboxTrioFixture()
	m := pagedModel(t, &failSecondTrash{Mock: mock})
	m, _ = step(m, pressKey("X"))
	m, _ = step(m, pressKey("#"))
	m, cmd := step(m, pressKey("s"))
	m = applyAll(t, m, cmd)
	if inTrash(mock) != 1 {
		t.Fatalf("nel cestino = %d, voglio 1 (la sola spostata prima dell'errore)", inTrash(mock))
	}
	if !toastHasUndo(m) {
		t.Fatalf("toast = %v, voglio l'offerta di u per la conversazione spostata", m.toast.Texts())
	}
	if !toastContains(m, "server giù") {
		t.Fatalf("toast = %v, voglio l'errore per le conversazioni non spostate", m.toast.Texts())
	}
	m, cmd = step(m, pressKey("u"))
	applyAll(t, m, cmd)
	if inTrash(mock) != 0 {
		t.Errorf("u: nel cestino = %d, voglio 0", inTrash(mock))
	}
	for uid := uint32(1); uid <= 3; uid++ {
		if !hasInbox(mock, uid) {
			t.Errorf("u: uid %d non è tornato in INBOX", uid)
		}
	}
}

// toastCount conta i toast il cui testo contiene s.
func toastCount(m Model, s string) int {
	n := 0
	for _, t := range m.toast.Texts() {
		if strings.Contains(t, s) {
			n++
		}
	}
	return n
}

// Piano 10, review punto 2 (ordine archivia → invio): l'invio sostituisce l'annulla
// dell'archivia, quindi il toast dell'archivia non deve più dire "u annulla" (u ora
// annulla l'invio). Il testo torna all'esito, senza l'offerta.
func TestArchiveToastDropsUndoHintWhenSendReplacesIt(t *testing.T) {
	mock := undoFixture(t, false)
	rec := &senderRecorder{}
	m := undoSenderModel(t, mock, rec)
	m, cmd := step(m, pressKey("a"))
	m = applyAll(t, m, cmd)
	if !toastContains(m, "archiviata — u annulla") {
		t.Fatalf("precondizione: toast = %v, voglio l'archivia con u", m.toast.Texts())
	}
	m = confirmDrained(t, sendRequested(t, writing(m)))
	if toastContains(m, "archiviata — u annulla") {
		t.Errorf("toast = %v: l'archivia non offre più u, ora annulla l'invio", m.toast.Texts())
	}
	if !toastContains(m, "archiviata") {
		t.Errorf("toast = %v, voglio l'esito \"archiviata\" senza offerta", m.toast.Texts())
	}
}

// Piano 10, review punto 2 (stesso meccanismo, archivia → archivia): il nuovo
// archivia sostituisce l'annulla del precedente, quindi solo UN toast offre u.
func TestSecondArchiveDropsUndoHintOfFirst(t *testing.T) {
	old := undoBulkWindow
	undoBulkWindow = time.Millisecond // il timer dell'annulla è eseguito dal runCmd dei test
	t.Cleanup(func() { undoBulkWindow = old })
	mock := inboxTrioFixture()
	for uid := uint32(1); uid <= 3; uid++ {
		mock.Msgs = append(mock.Msgs, &msg.Message{Ref: msg.MessageRef{
			ID: msg.NewID("[Gmail]/Tutti i messaggi", 100+uid), GmailID: uint64(10 + uid),
		}})
	}
	m := pagedModel(t, mock)
	m, cmd := step(m, pressKey("a"))
	m = settle(t, m, cmd, 4) // col refresh: il secondo archivia è sulla riga successiva
	m, cmd = step(m, pressKey("a"))
	m = settle(t, m, cmd, 4)
	if n := toastCount(m, "u annulla"); n != 1 {
		t.Errorf("toast con u = %d (%v), voglio 1: l'ultimo archivia è l'unico annullabile", n, m.toast.Texts())
	}
}

// modRec registra anche le label aggiunte e rimosse da ModifyLabels (opRec registra
// solo gli ID). Piano 10, review punto 3.
type modRec struct {
	*opRec
	adds [][]string
}

// ModifyLabels registra le label e delega a opRec.
func (r *modRec) ModifyLabels(ctx context.Context, ids []msg.MessageID, add, remove []string) error {
	r.adds = append(r.adds, append([]string(nil), add...))
	return r.opRec.ModifyLabels(ctx, ids, add, remove)
}

// convSentFixture: una conversazione (thread 50) con un messaggio in INBOX (uid 1,
// GmailID 11, label utente "Lavoro", il più recente) e uno in Inviati (uid 5, GmailID
// 12, il più vecchio). Ciascuno ha la sua copia in Tutti i messaggi (uid 9 e 10).
func convSentFixture() *mail.Mock {
	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	return &mail.Mock{
		Mailboxes: []mail.Mailbox{
			{Name: "INBOX", Role: "inbox"},
			{Name: "[Gmail]/Inviati", Role: "sent"},
			{Name: "[Gmail]/Tutti i messaggi", Role: "all"},
			{Name: "[Gmail]/Trash", Role: "trash"},
			{Name: "Lavoro"},
		},
		Msgs: []*msg.Message{
			{Ref: msg.MessageRef{ID: msg.NewID("INBOX", 1), GmailID: 11, ThreadID: 50,
				From: "anna@x.dev", Subject: "tema", Date: base.Add(2 * time.Hour), Labels: []string{"Lavoro"}}},
			{Ref: msg.MessageRef{ID: msg.NewID("[Gmail]/Inviati", 5), GmailID: 12, ThreadID: 50,
				From: "tu@x.dev", Subject: "Re: tema", Date: base.Add(time.Hour)}},
			{Ref: msg.MessageRef{ID: msg.NewID("[Gmail]/Tutti i messaggi", 9), GmailID: 11, ThreadID: 50,
				From: "anna@x.dev", Subject: "tema", Date: base.Add(2 * time.Hour), Labels: []string{"Lavoro"}}},
			{Ref: msg.MessageRef{ID: msg.NewID("[Gmail]/Tutti i messaggi", 10), GmailID: 12, ThreadID: 50,
				From: "tu@x.dev", Subject: "Re: tema", Date: base.Add(time.Hour)}},
		},
	}
}

// inboxHasGmailID dice se in INBOX c'è una copia con quel GmailID.
func inboxHasGmailID(mock *mail.Mock, gid uint64) bool {
	for _, mm := range mock.Msgs {
		if box, _, err := mm.Ref.ID.Split(); err == nil && box == "INBOX" && mm.Ref.GmailID == gid {
			return true
		}
	}
	return false
}

// Piano 10, review punto 3 (D10-T4): su una conversazione con un messaggio in Inviati,
// l'annulla dell'archivia rimette in INBOX SOLO il messaggio archiviato (ID di All Mail);
// il messaggio in Inviati non riceve \Inbox.
func TestArchiveUndoRestoresOnlyArchivedMessages(t *testing.T) {
	mock := convSentFixture()
	rec := &modRec{opRec: &opRec{MailStore: mock, mock: mock}}
	m := pagedModel(t, rec)
	m, cmd := step(m, pressKey("a"))
	m = applyAll(t, m, cmd)
	if len(rec.archived) != 1 || !sameIDs(rec.archived[0], msg.NewID("INBOX", 1)) {
		t.Fatalf("Archive = %v, voglio solo il messaggio in INBOX", rec.archived)
	}
	m, cmd = step(m, pressKey("u"))
	applyAll(t, m, cmd)
	if len(rec.modified) != 1 || !sameIDs(rec.modified[0], msg.NewID("[Gmail]/Tutti i messaggi", 9)) {
		t.Fatalf("ModifyLabels dell'annulla = %v, voglio la sola copia di All Mail dell'archiviato", rec.modified)
	}
	if len(rec.adds) != 1 || len(rec.adds[0]) != 1 || rec.adds[0][0] != `\Inbox` {
		t.Errorf("label aggiunte dell'annulla = %v, voglio [\\Inbox]", rec.adds)
	}
	if !inboxHasGmailID(mock, 11) {
		t.Error("u: il messaggio archiviato non è tornato in INBOX")
	}
	if inboxHasGmailID(mock, 12) {
		t.Error("u: il messaggio in Inviati è stato riportato in INBOX")
	}
}

// Piano 10, review punto 3 (D10-C2): l'annulla del cestino riporta nella cartella
// d'origine SOLO i messaggi cestinati e riapplica la label utente (non \Inbox).
func TestTrashUndoReappliesUserLabelsOnly(t *testing.T) {
	mock := convSentFixture()
	rec := &modRec{opRec: &opRec{MailStore: mock, mock: mock}}
	m := pagedModel(t, rec)
	m, _ = step(m, pressKey("#"))
	m, cmd := step(m, pressKey("s"))
	m = applyAll(t, m, cmd)
	if len(rec.trashed) != 1 || !sameIDs(rec.trashed[0], msg.NewID("INBOX", 1)) {
		t.Fatalf("Trash = %v, voglio solo il messaggio in INBOX", rec.trashed)
	}
	m, cmd = step(m, pressKey("u"))
	applyAll(t, m, cmd)
	if len(rec.adds) != 1 || len(rec.adds[0]) != 1 || rec.adds[0][0] != "Lavoro" {
		t.Errorf("label riapplicate = %v, voglio [Lavoro]", rec.adds)
	}
	if len(rec.modified) != 1 || len(rec.modified[0]) != 1 {
		t.Fatalf("ModifyLabels del ritorno = %v, voglio un solo ID ripristinato", rec.modified)
	}
	if box, _, err := rec.modified[0][0].Split(); err != nil || box != "INBOX" {
		t.Errorf("ModifyLabels del ritorno = %v, voglio un ID in INBOX", rec.modified[0])
	}
	if !inboxHasGmailID(mock, 11) {
		t.Error("u: il messaggio cestinato non è tornato in INBOX")
	}
	if inboxHasGmailID(mock, 12) {
		t.Error("u: il messaggio in Inviati è stato riportato in INBOX")
	}
}

// noIDsFirstTrash: la prima Trash sposta ma non restituisce gli ID nel cestino (server
// senza UIDPLUS: nessun annulla possibile); la seconda fallisce. Piano 10, review punto 5.
type noIDsFirstTrash struct {
	*mail.Mock
	calls int
}

// Trash delega la prima chiamata scartando gli ID; la seconda fallisce senza agire.
func (f *noIDsFirstTrash) Trash(ctx context.Context, ids []msg.MessageID) ([]msg.MessageID, error) {
	f.calls++
	if f.calls == 1 {
		_, err := f.Mock.Trash(ctx, ids)
		return nil, err
	}
	return nil, errors.New("server giù")
}

// Piano 10, review punto 5: errore a metà di un cestino senza annulla (il server non ha
// restituito gli ID). Le conversazioni effettivamente spostate escono comunque dalla
// lista: il loro esito è nel toast e non si rivedono in INBOX alla prossima lettura.
// Lo step è sull'opDone da solo, senza il refresh che lo seguirebbe.
func TestBulkTrashPartialErrorWithoutUndoRemovesMovedRows(t *testing.T) {
	mock := inboxTrioFixture()
	m := pagedModel(t, &noIDsFirstTrash{Mock: mock})
	m, _ = step(m, pressKey("X"))
	m, _ = step(m, pressKey("#"))
	m, cmd := step(m, pressKey("s"))
	od := opDoneOf(t, runCmd(cmd))
	if od.undo != nil {
		t.Fatalf("precondizione: undo = %+v, voglio nil (nessun ID nel cestino)", od.undo)
	}
	m, _ = step(m, od)
	moved := msg.NewID("INBOX", 3)
	for _, r := range m.refs {
		if r.ID == moved {
			t.Errorf("lista = %v: la conversazione spostata nel cestino è ancora visibile", m.refs)
		}
	}
	if !toastContains(m, "server giù") {
		t.Errorf("toast = %v, voglio l'errore del cestino", m.toast.Texts())
	}
}
