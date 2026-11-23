package app

// Piano 15, Task 16 (D15-11): menu `,`, pannello label `l` e selettore `m` sono
// finestre sovrapposte (internal/ui/overlay) centrate sopra la vista corrente.

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// assertWindowOver verifica che View mostri la finestra (titolo e bordo arrotondato)
// centrata, e che la lista dietro sia ancora visibile (non sostituita).
func assertWindowOver(t *testing.T, m Model, title string) {
	t.Helper()
	view := plainText(m.View().Content)
	lines := strings.Split(view, "\n")
	var tl string
	for _, l := range lines {
		if strings.Contains(l, title) {
			tl = l
			break
		}
	}
	if tl == "" {
		t.Fatalf("titolo %q assente dalla vista:\n%s", title, view)
	}
	if !strings.Contains(view, "╭") || !strings.Contains(view, "╯") {
		t.Errorf("la finestra non ha il bordo arrotondato:\n%s", view)
	}
	if x := strings.Index(tl, title); x < 10 {
		t.Errorf("finestra non centrata: titolo a colonna %d", x)
	}
	// La lista dietro resta: la prima riga con testo dell'inbox e ancora in vista.
	for _, l := range strings.Split(plainText(m.inbox.View()), "\n") {
		if s := strings.TrimSpace(l); s != "" {
			if !strings.Contains(view, s) {
				t.Errorf("la lista dietro la finestra e sparita (riga %q):\n%s", s, view)
			}
			break
		}
	}
}

func TestMenuImpostazioniEFinestraSovrapposta(t *testing.T) {
	m, _ := settingsModel(t)
	mm, _ := step(m, pressKey(","))
	assertWindowOver(t, mm, "impostazioni")
	if v := plainText(mm.View().Content); !strings.Contains(v, settingsHelp) {
		t.Errorf("riga di aiuto del menu assente:\n%s", v)
	}
	mm, _ = step(mm, pressKey("esc"))
	if mm.menu.open || strings.Contains(plainText(mm.View().Content), "impostazioni") {
		t.Error("esc non chiude la finestra")
	}
}

func TestSelettoreCartelleEFinestraSovrapposta(t *testing.T) {
	m := openPicker(t, folderModel(t, &recorder{MailStore: treeMock(t)}))
	assertWindowOver(t, m, "cartelle e label")
	if v := plainText(m.View().Content); !strings.Contains(v, "invio apre") {
		t.Errorf("hint dei tasti assente:\n%s", v)
	}
	m, _ = step(m, pressKey("esc"))
	if m.folderOpen {
		t.Error("esc non chiude il selettore")
	}
}

func TestPannelloLabelEFinestraSovrapposta(t *testing.T) {
	m := pagedModel(t, labelsFixture(t, false))
	m, _ = step(m, pressKey("X"))
	m, cmd := step(m, pressKey("l"))
	m = applyAll(t, m, cmd)
	assertWindowOver(t, m, "etichette su")
	if v := plainText(m.View().Content); !strings.Contains(v, "Lavoro") {
		t.Errorf("righe del pannello assenti:\n%s", v)
	}
}

// Con una finestra aperta il mouse e ignorato (il gating non cambia).
func TestMouseIgnoratoConFinestraAperta(t *testing.T) {
	m := openPicker(t, folderModel(t, &recorder{MailStore: treeMock(t)}))
	before := m.inbox.SelectedIndex()
	mm, cmd := step(m, tea.MouseClickMsg{X: 5, Y: 3, Button: tea.MouseLeft})
	if cmd != nil || mm.inbox.SelectedIndex() != before || !mm.folderOpen {
		t.Errorf("mouse con selettore aperto: cmd=%v cursore %d->%d", cmd, before, mm.inbox.SelectedIndex())
	}
}
