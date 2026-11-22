package app

import (
	"strings"
	"testing"
	"time"

	"github.com/andsigno82/gmail-tui/internal/ui/composer"
)

// Piano 9, review A (decisione di Andrea): alla conferma d'invio il messaggio è
// SALVATO SUBITO in Bozze, prima della finestra di 5 s; viene tolto da Bozze solo
// dopo l'invio riuscito. Il salvataggio gira nella cmd e il suo esito (sendSaved)
// fa partire la finestra.

// confirmDrained: ctrl+s già consegnato al radice; `s` nel modale e il salvataggio
// in Bozze eseguito (esito sendSaved elaborato): finestra attiva, composer chiuso.
func confirmDrained(t *testing.T, m Model) Model {
	t.Helper()
	mm, cmd := step(m, pressKey("s"))
	return drainOp(t, mm, cmd)
}

// TestConfirmSendSavesDraftBeforeWindow (review A, caso base e SIGINT): dopo la
// conferma la bozza è già in Bozze prima della scadenza, nessun invio parte.
func TestConfirmSendSavesDraftBeforeWindow(t *testing.T) {
	rec := &senderRecorder{}
	m, mk := senderModel(t, rec)
	mm, _ := step(m, teaSize())
	mm = sendRequested(t, writing(mm))
	mm, cmd := step(mm, pressKey("s"))
	if len(inDrafts(mk)) != 0 {
		t.Fatal("bozza in Bozze prima dell'esecuzione del salvataggio")
	}
	mm = drainOp(t, mm, cmd)
	if got := inDrafts(mk); len(got) != 1 {
		t.Fatalf("bozze in Bozze dopo la conferma = %v, voglio 1 (salvata prima della finestra)", got)
	}
	if mm.undo == nil {
		t.Fatal("finestra di annullamento non attiva dopo il salvataggio")
	}
	if len(rec.calls) != 0 {
		t.Errorf("invii durante la finestra = %d, voglio 0", len(rec.calls))
	}
	if mm.currentView() == viewComposer {
		t.Error("il composer deve chiudersi dopo il salvataggio")
	}
}

// TestUndoReopensSavedDraft (review A): `u` riapre il composer sulla bozza già in
// Bozze (stesso draftID), così esc la risalva sostituendo quella copia.
func TestUndoReopensSavedDraft(t *testing.T) {
	rec := &senderRecorder{}
	m, mk := senderModel(t, rec)
	mm, _ := step(m, teaSize())
	mm = confirmDrained(t, sendRequested(t, writing(mm)))
	saved := inDrafts(mk)
	if len(saved) != 1 {
		t.Fatalf("bozze = %v, voglio 1", saved)
	}
	mm, _ = step(mm, pressKey("u"))
	if mm.currentView() != viewComposer {
		t.Fatalf("vista dopo u = %v, voglio composer", mm.currentView())
	}
	if mm.draftID != saved[0] {
		t.Errorf("draftID dopo u = %q, voglio %q (la bozza già salvata)", mm.draftID, saved[0])
	}
}

// TestSendFailureReopensSavedDraft (review A, invio fallito): la bozza è in Bozze;
// il composer si riapre con quel draftID, senza duplicati.
func TestSendFailureReopensSavedDraft(t *testing.T) {
	rec := &senderRecorder{err: errBoom()}
	m, mk := senderModel(t, rec)
	mm, _ := step(m, teaSize())
	mm = confirmDrained(t, sendRequested(t, writing(mm)))
	mm, cmd := step(mm, undoDueMsg{seq: undoSeqOf(t, mm)})
	mm = drainOp(t, mm, cmd)
	saved := inDrafts(mk)
	if len(saved) != 1 {
		t.Fatalf("bozze dopo invio fallito = %v, voglio 1", saved)
	}
	if mm.currentView() != viewComposer || mm.draftID != saved[0] {
		t.Errorf("dopo invio fallito: vista %v draftID %q, voglio composer e %q", mm.currentView(), mm.draftID, saved[0])
	}
}

// TestSaveFailureStillArmsWindowWithWarning (review A): se il salvataggio iniziale
// fallisce l'invio resta possibile, con un toast d'avviso.
func TestSaveFailureStillArmsWindowWithWarning(t *testing.T) {
	rec := &senderRecorder{}
	m, mk := senderModel(t, rec)
	mk.Err = errBoom()
	mm, _ := step(m, teaSize())
	mm = confirmDrained(t, sendRequested(t, writing(mm)))
	if mm.undo == nil {
		t.Fatal("finestra non attiva dopo un salvataggio fallito: l'invio deve restare possibile")
	}
	if !strings.Contains(toastText(mm), "invio comunque possibile") {
		t.Errorf("toast = %q, voglio l'avviso \"invio comunque possibile\"", toastText(mm))
	}
	mm, cmd := step(mm, undoDueMsg{seq: undoSeqOf(t, mm)})
	mm = drainOp(t, mm, cmd)
	if len(rec.calls) != 1 {
		t.Errorf("invii = %d, voglio 1", len(rec.calls))
	}
}

// TestNewComposerDuringWindowKeepsFailedDraft (review A): un composer nuovo aperto
// durante la finestra non viene sovrascritto dal retry; il toast dice che il
// messaggio è rimasto in Bozze.
func TestNewComposerDuringWindowKeepsFailedDraft(t *testing.T) {
	rec := &senderRecorder{err: errBoom()}
	m, mk := senderModel(t, rec)
	mm, _ := step(m, teaSize())
	mm = confirmDrained(t, sendRequested(t, writing(mm)))
	mm = draftComposer(mm, composer.ComposerConfig{To: "nuovo@example.dev", Body: "nuovo"})
	mm, cmd := step(mm, undoDueMsg{seq: undoSeqOf(t, mm)})
	mm = drainOp(t, mm, cmd)
	if !strings.Contains(mm.composer.BodyText(), "nuovo") {
		t.Errorf("il composer nuovo è stato sostituito: corpo = %q", mm.composer.BodyText())
	}
	if !strings.Contains(toastText(mm), "rimasto in Bozze") {
		t.Errorf("toast = %q, voglio \"rimasto in Bozze\"", toastText(mm))
	}
	if len(inDrafts(mk)) != 1 {
		t.Errorf("bozze in Bozze = %v, voglio il messaggio fallito", inDrafts(mk))
	}
}

// TestQuitFlushesAndRemovesSavedDraft (review A): uscita con invio in attesa → il
// messaggio parte e la copia in Bozze viene tolta dopo l'invio riuscito.
func TestQuitFlushesAndRemovesSavedDraft(t *testing.T) {
	rec := &senderRecorder{}
	m, mk := senderModel(t, rec)
	mm, _ := step(m, teaSize())
	mm = confirmDrained(t, sendRequested(t, writing(mm)))
	_, cmd := step(mm, pressKey("ctrl+c"))
	runCmd(cmd)
	if len(rec.calls) != 1 {
		t.Errorf("invii alla chiusura = %d, voglio 1", len(rec.calls))
	}
	if got := inDrafts(mk); len(got) != 0 {
		t.Errorf("bozze dopo l'invio in uscita = %v, voglio 0", got)
	}
}

// undoSeqOf restituisce il seq dell'invio in attesa (fallisce il test se manca).
func undoSeqOf(t *testing.T, m Model) int {
	t.Helper()
	if m.undo == nil {
		t.Fatal("nessun invio in attesa della finestra di annullamento")
	}
	return m.undo.seq
}

// TestQuitReportsFailedSendOnExit (review A): invio in uscita fallito → il
// messaggio resta in Bozze e l'esito va agli esiti d'uscita (stderr dopo la TUI).
func TestQuitReportsFailedSendOnExit(t *testing.T) {
	rec := &senderRecorder{err: errBoom()}
	m, _ := senderModel(t, rec)
	mm, _ := step(m, teaSize())
	mm = confirmDrained(t, sendRequested(t, writing(mm)))
	_, cmd := step(mm, pressKey("ctrl+c"))
	runCmd(cmd)
	notes := strings.Join(mm.ExitMessages(), "\n")
	if !strings.Contains(notes, "invio non riuscito") || !strings.Contains(notes, "rimasto in Bozze") {
		t.Errorf("esiti d'uscita = %q, voglio invio fallito e messaggio rimasto in Bozze", notes)
	}
}

// TestQuitWithUnconfirmedSaveDoesNotSend (review A): se il salvataggio in Bozze non
// si conclude in tempo in uscita, nulla parte e non si tocca Bozze alla cieca.
func TestQuitWithUnconfirmedSaveDoesNotSend(t *testing.T) {
	old := saveWait
	saveWait = 20 * time.Millisecond
	defer func() { saveWait = old }()
	rec := &senderRecorder{}
	m, _ := senderModel(t, rec)
	mm, _ := step(m, teaSize())
	mm = sendRequested(t, writing(mm))
	mm, _ = step(mm, pressKey("s")) // salvataggio NON eseguito: la cmd non parte mai
	_, cmd := step(mm, pressKey("ctrl+c"))
	runCmd(cmd)
	if len(rec.calls) != 0 {
		t.Errorf("invii con salvataggio non confermato = %d, voglio 0", len(rec.calls))
	}
	if !strings.Contains(strings.Join(mm.ExitMessages(), "\n"), "non confermato") {
		t.Errorf("esiti d'uscita = %v, voglio l'avviso di salvataggio non confermato", mm.ExitMessages())
	}
}
