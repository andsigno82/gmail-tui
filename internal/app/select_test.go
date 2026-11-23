package app

import (
	"strings"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// selectFixture: la INBOX di convOpsFixture ha due righe (la conversazione 7 e il
// messaggio singolo INBOX:4); la cartella Inviati fa da destinazione del cambio.
func selectFixture(t *testing.T) Model {
	t.Helper()
	return pagedModel(t, convOpsFixture(t))
}

// Piano 10, Task 2: `x` marca e smarca la riga sotto il cursore (D10-S1).
func TestXTogglesRowUnderCursor(t *testing.T) {
	m := selectFixture(t)
	id := msg.NewID("INBOX", 3) // Latest della conversazione 7
	m, _ = step(m, pressKey("x"))
	if !m.sel[id] || len(m.sel) != 1 {
		t.Fatalf("dopo x: selezione = %v, voglio solo la riga del cursore %s", m.sel, id)
	}
	m, _ = step(m, pressKey("x"))
	if len(m.sel) != 0 {
		t.Errorf("secondo x: selezione = %v, voglio vuota", m.sel)
	}
}

// Piano 10, Task 2: `X` seleziona tutte le righe caricate; di nuovo le azzera (D10-S1).
func TestXCapitalSelectsAllLoaded(t *testing.T) {
	m := selectFixture(t)
	m, _ = step(m, pressKey("X"))
	if len(m.sel) != 2 {
		t.Fatalf("dopo X: %d righe selezionate, voglio 2", len(m.sel))
	}
	m, _ = step(m, pressKey("X"))
	if len(m.sel) != 0 {
		t.Errorf("secondo X: selezione = %v, voglio vuota", m.sel)
	}
}

// Piano 10, Task 2: con selezione attiva `esc` azzera la selezione senza uscire
// dall'app (il cmd di uscita è nil).
func TestEscClearsSelectionInInbox(t *testing.T) {
	m := selectFixture(t)
	m, _ = step(m, pressKey("X"))
	m, cmd := step(m, pressKey("esc"))
	if len(m.sel) != 0 {
		t.Errorf("esc: selezione = %v, voglio vuota", m.sel)
	}
	if cmd != nil {
		t.Errorf("esc con selezione ha avviato un comando (uscita?)")
	}
	if m.currentView() != viewInbox {
		t.Errorf("vista = %v, voglio viewInbox", m.currentView())
	}
}

// Piano 10, Task 2 (D10-S3): cambiando cartella la selezione si azzera.
func TestSelectionResetOnFolderChange(t *testing.T) {
	m := pagedModel(t, folderMock(t)) // INBOX + Inviati, cartelle caricate al selettore
	m, _ = step(m, pressKey("X"))
	if len(m.sel) == 0 {
		t.Fatalf("precondizione: nessuna riga selezionata")
	}
	m, cmd := step(m, pressKey("m"))
	m = applyAll(t, m, cmd)
	m.folderList.Select(1) // [Gmail]/Sent
	m, cmd = step(m, pressKey("enter"))
	m = applyAll(t, m, cmd)
	if m.folder.Name != "[Gmail]/Sent" {
		t.Fatalf("cartella = %q, voglio [Gmail]/Sent: il cambio non è avvenuto", m.folder.Name)
	}
	if len(m.sel) != 0 {
		t.Errorf("cambio cartella: selezione = %v, voglio vuota", m.sel)
	}
}

// Piano 10, Task 2 (D10-S3): una nuova ricerca azzera la selezione.
func TestSelectionResetOnSearch(t *testing.T) {
	m := selectFixture(t)
	m, _ = step(m, pressKey("X"))
	if len(m.sel) == 0 {
		t.Fatalf("precondizione: nessuna riga selezionata")
	}
	m, _ = step(m, pressKey("/"))
	m.search.SetValue("discussione")
	m, cmd := step(m, pressKey("enter"))
	m = applyAll(t, m, cmd)
	if len(m.sel) != 0 {
		t.Errorf("ricerca: selezione = %v, voglio vuota", m.sel)
	}
}

// Piano 10, Task 2: la barra di stato dice quante righe sono selezionate, e
// non dice nulla senza selezione.
func TestStatusBarShowsSelectedCount(t *testing.T) {
	m := selectFixture(t)
	if strings.Contains(m.statusBar(), "selezionate") {
		t.Errorf("senza selezione la barra non deve dire selezionate: %q", m.statusBar())
	}
	m, _ = step(m, pressKey("X"))
	if !strings.Contains(m.statusBar(), "2 selezionate") {
		t.Errorf("barra = %q, voglio 2 selezionate", m.statusBar())
	}
}

// Piano 10, Task 2 (selezione, passo 1): un refresh che fa sparire una riga la
// toglie dalla selezione; il conteggio "N selezionate" non resta stantio.
func TestSelectionPrunedWhenRowDisappears(t *testing.T) {
	m := selectFixture(t)
	m, _ = step(m, pressKey("X"))
	if len(m.sel) != 2 {
		t.Fatalf("precondizione: %d righe selezionate, voglio 2", len(m.sel))
	}
	keep := m.refs[:1]
	gone := m.refs[1].ID
	m, _ = step(m, msgsLoaded{refs: keep, silent: true, gen: m.listGen, mailbox: m.folder.Name, limit: pageSize})
	if len(m.sel) != 1 || !m.sel[keep[0].ID] {
		t.Fatalf("dopo il refresh: selezione = %v, voglio solo %s", m.sel, keep[0].ID)
	}
	if m.sel[gone] {
		t.Errorf("la riga sparita %s è ancora selezionata", gone)
	}
}
