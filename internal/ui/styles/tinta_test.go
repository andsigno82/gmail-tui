package styles

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"

	"github.com/andsigno82/gmail-tui/internal/theme"
)

// TestMescolaSRGB (richiesta di Andrea, 2026-11-23): la tinta della riga spuntata
// è la mescolanza lineare in sRGB fra bg e selection, calcolata per variante
// dark e light separatamente. Frazione 0 dà bg, frazione 1 dà selection.
func TestMescolaSRGB(t *testing.T) {
	cases := []struct {
		name               string
		bg, sel            theme.Slot
		frac               float64
		wantDark, wantLite string
	}{
		{"20% nero verso bianco", slotHex("#000000"), slotHex("#ffffff"), 0.20, "#333333", "#333333"},
		{"50% arrotonda", slotHex("#000000"), slotHex("#ffffff"), 0.50, "#808080", "#808080"},
		{"0 dà bg", slotHex("#000000"), slotHex("#ffffff"), 0, "#000000", "#000000"},
		{"1 dà selection", slotHex("#000000"), slotHex("#ffffff"), 1, "#ffffff", "#ffffff"},
		{
			"varianti dark e light separate",
			theme.Slot{Dark: "#000000", Light: "#ffffff"},
			theme.Slot{Dark: "#ffffff", Light: "#000000"},
			0.20, "#333333", "#cccccc",
		},
		{
			"tema default (dark #16161e->#2e2e42, light #ffffff->#ddf4ff)",
			theme.Slot{Dark: "#16161e", Light: "#ffffff"},
			theme.Slot{Dark: "#2e2e42", Light: "#ddf4ff"},
			0.20, "#1b1b25", "#f8fdff",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := mescola(c.bg, c.sel, c.frac)
			if !strings.EqualFold(got.Dark, c.wantDark) {
				t.Errorf("variante dark = %s, voglio %s", got.Dark, c.wantDark)
			}
			if !strings.EqualFold(got.Light, c.wantLite) {
				t.Errorf("variante light = %s, voglio %s", got.Light, c.wantLite)
			}
		})
	}
}

// TestTintaSelezioneSfondo: gli stili della riga spuntata hanno come sfondo la
// tinta calcolata da bg e selection (non lo slot selection pieno) in entrambe le
// varianti, per ogni tema embedded.
func TestTintaSelezioneSfondo(t *testing.T) {
	for _, name := range theme.Names() {
		t.Run(name, func(t *testing.T) {
			th := loadTheme(t, name)
			tint := mescola(th.Bg, th.Selection, frazioneTinta)
			tD, tL := tint.Resolve()
			s := New(th)
			for _, st := range []lipgloss.Style{s.ListTint, s.ListTintUnread, s.ListTintStarred, s.ListTintDate, s.ListTintCheck} {
				bg, ok := st.GetBackground().(compat.AdaptiveColor)
				if !ok {
					t.Fatal("sfondo della tinta non adattivo")
				}
				if !sameColor(bg.Dark, lipgloss.Color(tD)) || !sameColor(bg.Light, lipgloss.Color(tL)) {
					t.Errorf("sfondo = %v/%v, voglio tinta %s/%s", bg.Dark, bg.Light, tD, tL)
				}
			}
		})
	}
}

// TestEtichetteTintaContrasta: il palette delle etichette su riga spuntata
// (LabelsTint) ha stesso numero e ordine di Labels, e ogni etichetta ha contrasto
// >= 4.5:1 sulla tinta in entrambe le varianti.
func TestEtichetteTintaContrasta(t *testing.T) {
	for _, name := range theme.Names() {
		t.Run(name, func(t *testing.T) {
			th := loadTheme(t, name)
			s := New(th)
			if len(s.LabelsTint) != len(s.Labels) || len(s.LabelsTint) == 0 {
				t.Fatalf("LabelsTint = %d, Labels = %d", len(s.LabelsTint), len(s.Labels))
			}
			tD, tL := mescola(th.Bg, th.Selection, frazioneTinta).Resolve()
			for i, st := range s.LabelsTint {
				fg, ok := st.GetForeground().(compat.AdaptiveColor)
				if !ok {
					t.Fatalf("etichetta %d: testo non adattivo", i)
				}
				if r := contrastRatio(fg.Dark, lipgloss.Color(tD)); r < minContrast {
					t.Errorf("etichetta %d dark: contrasto %.2f < %.1f", i, r, minContrast)
				}
				if r := contrastRatio(fg.Light, lipgloss.Color(tL)); r < minContrast {
					t.Errorf("etichetta %d light: contrasto %.2f < %.1f", i, r, minContrast)
				}
			}
		})
	}
}

// slotHex è uno slot con lo stesso colore nelle due varianti.
func slotHex(hex string) theme.Slot { return theme.Slot{Dark: hex, Light: hex} }
