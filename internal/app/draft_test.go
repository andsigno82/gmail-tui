package app

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/composer"
	"github.com/andsigno82/gmail-tui/internal/ui/toast"
)

// Piano 9, Task 5-7: bozze (esc salva, ctrl+d scarta, ripresa, risalvataggio,
// rimozione dopo l'invio). Nei test il toast non si esegue mai: il suo Tick
// dura 3 s, quindi si controlla solo il testo in pila.

// drainOp esegue la cmd di un'operazione (un solo Msg per cmd) e riapplica il risultato.
func drainOp(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	for _, ms := range runCmd(cmd) {
		m, _ = step(m, ms)
	}
	return m
}

// draftMsg è una bozza nel Mock (Piano 9): mailbox Bozze, con destinatari e corpo.
func draftMsg(uid uint32, to []string, subj, body string) *msg.Message {
	return &msg.Message{
		Ref:      msg.MessageRef{ID: msg.NewID("[Gmail]/Drafts", uid), Subject: subj, Date: time.Date(2026, 11, 22, 9, 0, 0, 0, time.UTC)},
		To:       to,
		BodyText: body,
	}
}

// inDrafts conta le bozze nella cartella Bozze del Mock.
func inDrafts(mk *mail.Mock) []msg.MessageID {
	var ids []msg.MessageID
	for _, mm := range mk.Msgs {
		if strings.HasPrefix(string(mm.Ref.ID), "[Gmail]/Drafts:") {
			ids = append(ids, mm.Ref.ID)
		}
	}
	return ids
}

// writing porta il radice in composer con un corpo (A, oggetto, corpo).
func writing(m Model) Model {
	return draftComposer(m, composer.ComposerConfig{To: "anna@example.dev", Subject: "ciao", Body: "testo"})
}

// TestEscSavesDraftAndClosesComposer: esc con bozza non vuota → APPEND in Bozze,
// toast "bozza salvata", ritorno alla vista di provenienza (D9-A).
func TestEscSavesDraftAndClosesComposer(t *testing.T) {
	m, mk := composerModel(t)
	mm, _ := step(m, teaSize())
	mm = writing(mm)
	mm, cmd := step(mm, pressKey("esc"))
	mm = drainOp(t, mm, cmd)
	if got := inDrafts(mk); len(got) != 1 {
		t.Fatalf("bozze in Bozze = %v, voglio 1", got)
	}
	if mm.currentView() != viewInbox {
		t.Errorf("vista = %v, voglio inbox (provenienza)", mm.currentView())
	}
	if !toastIs(mm, toast.Success) || !strings.Contains(toastText(mm), "bozza salvata") {
		t.Errorf("toast = %q, voglio \"bozza salvata\"", toastText(mm))
	}
}

// TestEscEmptyDraftNoSave: composer vuoto → nessuna bozza, nessun toast (D9-3).
func TestEscEmptyDraftNoSave(t *testing.T) {
	m, mk := composerModel(t)
	mm, _ := step(m, teaSize())
	mm = draftComposer(mm, composer.ComposerConfig{})
	mm, cmd := step(mm, pressKey("esc"))
	if cmd != nil {
		mm = drainOp(t, mm, cmd)
	}
	if got := inDrafts(mk); len(got) != 0 {
		t.Errorf("bozza vuota salvata: %v", got)
	}
	if mm.currentView() != viewInbox {
		t.Errorf("vista = %v, voglio inbox", mm.currentView())
	}
}

// TestEscKeepsComposerOnSaveError: errore del backend → composer aperto con la bozza, toast d'errore.
func TestEscKeepsComposerOnSaveError(t *testing.T) {
	m, mk := composerModel(t)
	mm, _ := step(m, teaSize())
	mm = writing(mm)
	mk.Err = errBoom()
	mm, cmd := step(mm, pressKey("esc"))
	mm = drainOp(t, mm, cmd)
	if mm.currentView() != viewComposer {
		t.Errorf("vista = %v, voglio composer aperto", mm.currentView())
	}
	if !toastIs(mm, toast.Error) {
		t.Errorf("toast = %q, voglio errore", toastText(mm))
	}
	if mm.draftBusy {
		t.Error("draftBusy resta acceso dopo l'errore")
	}
}

// TestCtrlDAsksDiscardConfirm: ctrl+d apre la conferma, il composer resta.
func TestCtrlDAsksDiscardConfirm(t *testing.T) {
	m, _ := composerModel(t)
	mm, _ := step(m, teaSize())
	mm = writing(mm)
	mm, _ = step(mm, pressKey("ctrl+d"))
	if !mm.discardOpen {
		t.Fatal("ctrl+d: conferma scarto non aperta")
	}
	if mm.currentView() != viewComposer {
		t.Errorf("vista = %v, voglio composer", mm.currentView())
	}
}

// TestCtrlDDiscardDeletesSavedDraft: ripresa di una bozza + ctrl+d + s → la bozza esce da Bozze.
func TestCtrlDDiscardDeletesSavedDraft(t *testing.T) {
	m, mk := composerModel(t, draftMsg(1, []string{"anna@example.dev"}, "ciao", "testo"))
	mm, _ := step(m, teaSize())
	mm.folder = mail.Mailbox{Name: "[Gmail]/Drafts", Role: "drafts"}
	mm, cmd := step(mm, inboxSelected(draftMsg(1, nil, "ciao", "").Ref))
	mm = drainOp(t, mm, cmd)
	mm, _ = step(mm, pressKey("ctrl+d"))
	mm, cmd = step(mm, pressKey("s"))
	mm = drainOp(t, mm, cmd)
	if got := inDrafts(mk); len(got) != 0 {
		t.Errorf("bozza ancora in Bozze dopo lo scarto: %v", got)
	}
	if mm.currentView() == viewComposer {
		t.Error("composer ancora aperto dopo lo scarto")
	}
}

// TestEnterInDraftsResumesComposer: invio su una riga di Bozze riapre il composer
// con destinatari, oggetto e corpo; draftID = ID della bozza (D9-D).
func TestEnterInDraftsResumesComposer(t *testing.T) {
	m, _ := composerModel(t, draftMsg(1, []string{"anna@example.dev"}, "ciao", "testo della bozza"))
	mm, _ := step(m, teaSize())
	mm.folder = mail.Mailbox{Name: "[Gmail]/Drafts", Role: "drafts"}
	ref := draftMsg(1, nil, "ciao", "").Ref
	mm, cmd := step(mm, inboxSelected(ref))
	mm = drainOp(t, mm, cmd)
	if mm.currentView() != viewComposer {
		t.Fatalf("vista = %v, voglio composer", mm.currentView())
	}
	d := mm.composer.Draft()
	if strings.Join(d.To, ",") != "anna@example.dev" || d.Subject != "ciao" || !strings.HasPrefix(d.BodyText, "testo della bozza") {
		t.Errorf("composer ripreso = %+v", d)
	}
	if mm.draftID != ref.ID {
		t.Errorf("draftID = %q, voglio %q", mm.draftID, ref.ID)
	}
}

// TestResumeStripsSignature: la firma salvata in coda non compare due volte.
func TestResumeStripsSignature(t *testing.T) {
	m, _ := composerModel(t, draftMsg(1, []string{"anna@example.dev"}, "ciao", "ciao\n\n--\n"+testSig))
	mm, _ := step(m, teaSize())
	mm.folder = mail.Mailbox{Name: "[Gmail]/Drafts", Role: "drafts"}
	mm, cmd := step(mm, inboxSelected(draftMsg(1, nil, "", "").Ref))
	mm = drainOp(t, mm, cmd)
	if got := mm.composer.BodyText(); got != "ciao" {
		t.Errorf("corpo ripreso = %q, voglio \"ciao\" (firma tolta, verrà riaggiunta)", got)
	}
}

// TestResavePreservesSingleCopy: riprendere e esc → in Bozze resta una sola bozza (la nuova).
func TestResavePreservesSingleCopy(t *testing.T) {
	m, mk := composerModel(t, draftMsg(1, []string{"anna@example.dev"}, "ciao", "testo"))
	mm, _ := step(m, teaSize())
	mm.folder = mail.Mailbox{Name: "[Gmail]/Drafts", Role: "drafts"}
	mm, cmd := step(mm, inboxSelected(draftMsg(1, nil, "", "").Ref))
	mm = drainOp(t, mm, cmd)
	mm, cmd = step(mm, pressKey("esc"))
	mm = drainOp(t, mm, cmd)
	got := inDrafts(mk)
	if len(got) != 1 || got[0] == msg.NewID("[Gmail]/Drafts", 1) {
		t.Errorf("bozze in Bozze = %v, voglio una sola bozza nuova", got)
	}
	if mm.draftID != "" && mm.currentView() == viewComposer {
		t.Error("composer ancora aperto dopo il salvataggio")
	}
}

// TestSendFromResumedDraftRemovesFromDrafts: dopo l'invio di una bozza ripresa
// la bozza esce da Bozze e Sent contiene il messaggio (D9-E).
func TestSendFromResumedDraftRemovesFromDrafts(t *testing.T) {
	rec := &senderRecorder{}
	m, mk := senderModel(t, rec, draftMsg(1, []string{"anna@example.dev"}, "ciao", "testo"))
	mm, _ := step(m, teaSize())
	mm.folder = mail.Mailbox{Name: "[Gmail]/Drafts", Role: "drafts"}
	mm, cmd := step(mm, inboxSelected(draftMsg(1, nil, "", "").Ref))
	mm = drainOp(t, mm, cmd)
	mm = sendRequested(t, mm)
	mm, _ = step(mm, pressKey("s"))
	mm, _ = flushUndo(t, mm)
	if len(rec.calls) != 1 {
		t.Fatalf("invii = %d, voglio 1", len(rec.calls))
	}
	if got := inDrafts(mk); len(got) != 0 {
		t.Errorf("bozza ancora in Bozze dopo l'invio: %v", got)
	}
}

// TestSendFailureKeepsDraft: invio fallito → la bozza resta in Bozze.
func TestSendFailureKeepsDraft(t *testing.T) {
	rec := &senderRecorder{err: errBoom()}
	m, mk := senderModel(t, rec, draftMsg(1, []string{"anna@example.dev"}, "ciao", "testo"))
	mm, _ := step(m, teaSize())
	mm.folder = mail.Mailbox{Name: "[Gmail]/Drafts", Role: "drafts"}
	mm, cmd := step(mm, inboxSelected(draftMsg(1, nil, "", "").Ref))
	mm = drainOp(t, mm, cmd)
	mm = sendRequested(t, mm)
	mm, _ = step(mm, pressKey("s"))
	mm, _ = flushUndo(t, mm)
	if got := inDrafts(mk); len(got) != 1 {
		t.Errorf("bozze dopo invio fallito = %v, voglio la bozza intatta", got)
	}
}

// --- annulla invio (Piano 9, Task 9, D9-I) -------------------------------------

// confirmSendAndUndoWindow: composer con corpo, ctrl+s, s → finestra di annullamento.
func confirmSendAndUndoWindow(t *testing.T, sender Sender) (Model, *mail.Mock) {
	t.Helper()
	m, mk := senderModel(t, sender)
	mm, _ := step(m, teaSize())
	mm = sendRequested(t, writing(mm))
	mm, _ = step(mm, pressKey("s"))
	return mm, mk
}

// flushUndo fa scadere la finestra di annullamento (undoDueMsg col seq del
// pendente, senza attendere i 5 s) ed esegue l'invio: restituisce il modello e
// l'opDone prodotto dal sender.
func flushUndo(t *testing.T, m Model) (Model, opDone) {
	t.Helper()
	if m.undo == nil {
		t.Fatal("nessun invio in attesa della finestra di annullamento")
	}
	mm, cmd := step(m, undoDueMsg{seq: m.undo.seq})
	var done opDone
	found := false
	for _, ms := range runCmd(cmd) {
		if od, ok := ms.(opDone); ok {
			done, found = od, true
		}
	}
	if !found {
		t.Fatal("nessun opDone dall'invio allo scadere della finestra")
	}
	return mm, done
}

// TestConfirmSendWaitsFiveSeconds: `s` non invia subito; il timer scaduto invia.
func TestConfirmSendWaitsFiveSeconds(t *testing.T) {
	rec := &senderRecorder{}
	mm, _ := confirmSendAndUndoWindow(t, rec)
	if len(rec.calls) != 0 {
		t.Fatalf("invio immediato con %d chiamate, voglio 0 durante la finestra", len(rec.calls))
	}
	if mm.undo == nil {
		t.Fatal("finestra di annullamento non attiva")
	}
	if !strings.Contains(toastText(mm), "invio tra 5 s — u annulla") {
		t.Errorf("toast = %q, voglio \"invio tra 5 s — u annulla\"", toastText(mm))
	}
	mm, cmd := step(mm, undoDueMsg{seq: mm.undo.seq})
	mm = drainOp(t, mm, cmd)
	if len(rec.calls) != 1 {
		t.Errorf("dopo la scadenza: invii = %d, voglio 1", len(rec.calls))
	}
	if mm.undo != nil {
		t.Error("undo ancora pendente dopo l'invio")
	}
}

// TestUndoRestoresComposer: `u` riporta il composer con lo stesso corpo e il
// timer successivo, stantio, non invia.
func TestUndoRestoresComposer(t *testing.T) {
	rec := &senderRecorder{}
	mm, _ := confirmSendAndUndoWindow(t, rec)
	seq := mm.undo.seq
	mm, _ = step(mm, pressKey("u"))
	if mm.currentView() != viewComposer {
		t.Fatalf("vista = %v dopo u, voglio composer", mm.currentView())
	}
	if !strings.HasPrefix(mm.composer.BodyText(), "testo") {
		t.Errorf("corpo ripristinato = %q, voglio \"testo\"", mm.composer.BodyText())
	}
	mm, cmd := step(mm, undoDueMsg{seq: seq})
	mm = drainOp(t, mm, cmd)
	if len(rec.calls) != 0 {
		t.Errorf("timer stantio ha inviato %d volte, voglio 0", len(rec.calls))
	}
}

// TestUndoIgnoredInsideComposer: `u` digitato nel composer è testo, l'invio resta in attesa.
func TestUndoIgnoredInsideComposer(t *testing.T) {
	rec := &senderRecorder{}
	mm, _ := confirmSendAndUndoWindow(t, rec)
	mm = writing(mm) // nuovo composer aperto durante la finestra
	mm, _ = step(mm, pressKey("u"))
	if !strings.Contains(mm.composer.BodyText(), "u") {
		t.Errorf("corpo = %q, voglio la u digitata", mm.composer.BodyText())
	}
	if mm.undo == nil {
		t.Error("la u nel composer ha annullato l'invio in attesa")
	}
}

// TestQuitFlushesPendingSend: q durante l'attesa → l'invio parte prima del QuitMsg.
func TestQuitFlushesPendingSend(t *testing.T) {
	rec := &senderRecorder{}
	mm, _ := confirmSendAndUndoWindow(t, rec)
	_, cmd := step(mm, pressKey("q"))
	if cmd == nil {
		t.Fatal("q: cmd nil")
	}
	var gotQuit bool
	for _, ms := range runCmd(cmd) {
		if _, ok := ms.(tea.QuitMsg); ok {
			gotQuit = true
		}
	}
	if !gotQuit {
		t.Fatal("q: nessun QuitMsg")
	}
	if len(rec.calls) != 1 {
		t.Errorf("invii alla chiusura = %d, voglio 1 (flush prima dell'uscita)", len(rec.calls))
	}
}

// TestDemoDraftsFolderHasSample: la demo ha la cartella Bozze (ruolo drafts) con
// una bozza d'esempio, così `m` → Bozze mostra la ripresa (Piano 9, Task 13).
func TestDemoDraftsFolderHasSample(t *testing.T) {
	mk := DemoStore()
	boxes, err := mk.ListMailboxes(context.Background())
	if err != nil {
		t.Fatalf("ListMailboxes: %v", err)
	}
	hasDrafts := false
	for _, b := range boxes {
		if b.Role == "drafts" {
			hasDrafts = true
		}
	}
	if !hasDrafts {
		t.Fatal("demo senza cartella Bozze (ruolo drafts)")
	}
	n := 0
	for _, mm := range mk.Msgs {
		if strings.HasPrefix(string(mm.Ref.ID), "[Gmail]/Drafts:") {
			n++
		}
	}
	if n == 0 {
		t.Error("demo: nessuna bozza d'esempio in Bozze")
	}
}

// TestUndoKeyOnlyInInboxAndThread (Piano 9, review E): `u` annulla l'invio solo
// in inbox e thread. In ricerca, in un prompt o in un selettore è testo/tasto
// della vista: l'invio resta in attesa e il composer non si riapre.
func TestUndoKeyOnlyInInboxAndThread(t *testing.T) {
	rec := &senderRecorder{}
	mm, _ := confirmSendAndUndoWindow(t, rec)
	seq := mm.undo.seq
	mm, _ = step(mm, pressKey("/")) // ricerca: la u è testo della query
	mm, _ = step(mm, pressKey("u"))
	if mm.currentView() == viewComposer {
		t.Error("u in ricerca ha riaperto il composer")
	}
	if mm.undo == nil || mm.undo.seq != seq {
		t.Error("u in ricerca ha annullato l'invio in attesa")
	}
}
