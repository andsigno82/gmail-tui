package app

// Piano 15, Task 16 (D15-12): la barra di stato e a segmenti (modo, cartella, stato,
// aiuto breve); le conferme restano barre colorate.

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestBarraSegmentiModoCartellaAiuto(t *testing.T) {
	m, _ := settingsModel(t)
	bar := plainBar(m)
	for _, want := range []string{"LISTA", "Posta in arrivo", "q"} {
		if !strings.Contains(bar, want) {
			t.Errorf("barra = %q, manca %q", bar, want)
		}
	}
	if w := lipgloss.Width(m.statusBar()); w != m.width {
		t.Errorf("larghezza barra = %d, voglio %d", w, m.width)
	}
	if strings.Contains(bar, "[") {
		t.Errorf("il tag [cartella] e sostituito dal segmento: %q", bar)
	}
}

func TestBarraSegmentoModoLettura(t *testing.T) {
	m := openThread(t, newTestModel(t, fullMessage(testRefs(t)[0])), testRefs(t)[0])
	if m.currentView() != viewThread {
		t.Fatalf("vista = %v", m.currentView())
	}
	if bar := plainBar(m); !strings.Contains(bar, "LETTURA") {
		t.Errorf("barra = %q, voglio il modo LETTURA", bar)
	}
}

func TestBarraSegmentoStatoAvvisoESelezione(t *testing.T) {
	m := selectFixture(t)
	m.SetStartupWarning("tema di ripiego")
	m, _ = step(m, pressKey("X"))
	bar := plainBar(m)
	for _, want := range []string{"2 selezionate", "tema di ripiego"} {
		if !strings.Contains(bar, want) {
			t.Errorf("barra = %q, manca %q", bar, want)
		}
	}
}

// Con lo spinner (op in volo) la barra resta larga quanto il terminale e non porta
// sequenze di stile dentro i segmenti oltre al loro stile.
func TestBarraConSpinnerLarghezzaEsatta(t *testing.T) {
	m, _ := settingsModel(t)
	m.pending = 1
	if w := lipgloss.Width(m.statusBar()); w != m.width {
		t.Errorf("larghezza con spinner = %d, voglio %d", w, m.width)
	}
}

// Le conferme restano barre colorate a tutta larghezza (StatusErr), non segmenti.
func TestConfermeRestanoBarreColorate(t *testing.T) {
	m := selectFixture(t)
	m.trashOpen = true
	bar := m.statusBar()
	if !strings.Contains(plainText(bar), "spostare nel cestino? s conferma") || strings.Contains(plainText(bar), "LISTA") {
		t.Errorf("barra di conferma = %q", plainText(bar))
	}
}
