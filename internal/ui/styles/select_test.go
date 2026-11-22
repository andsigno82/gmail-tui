package styles

import (
	"testing"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"

	"github.com/andsigno82/gmail-tui/internal/theme"
)

// TestCasellaSelezioneContrasta (Piano 10, Task 2): la casella ☐/☑ senza sfondo
// (ListCheck) deve restare a 4.5:1 sullo sfondo del tema, in entrambe le varianti.
// La casella sulla riga selezionata (ListCheckSel) ha sfondo e la copre
// TestContrastoStiliConSfondo.
func TestCasellaSelezioneContrasta(t *testing.T) {
	for _, name := range theme.Names() {
		t.Run(name, func(t *testing.T) {
			th := loadTheme(t, name)
			fg, ok := New(th).ListCheck.GetForeground().(compat.AdaptiveColor)
			if !ok {
				t.Fatal("casella: testo non adattivo (colore non calcolato)")
			}
			bgD, bgL := th.Bg.Resolve()
			if r := contrastRatio(fg.Dark, lipgloss.Color(bgD)); r < minContrast {
				t.Errorf("casella dark: contrasto %.2f < %.1f", r, minContrast)
			}
			if r := contrastRatio(fg.Light, lipgloss.Color(bgL)); r < minContrast {
				t.Errorf("casella light: contrasto %.2f < %.1f", r, minContrast)
			}
		})
	}
}
