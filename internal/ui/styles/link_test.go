package styles

import (
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/andsigno82/gmail-tui/internal/theme"
)

// TestContrastoLink (Richiesta di Andrea, 2026-11-23): lo stile del link usa lo
// slot accent sottolineato e il testo ha contrasto >= 4.5:1 sul bg del tema, in
// entrambe le varianti. Lo stile non ha sfondo: il test tabellare di
// TestContrastoStiliConSfondo non lo copre, quindi si verifica qui.
func TestContrastoLink(t *testing.T) {
	const minContrast = 4.5
	for _, name := range theme.Names() {
		t.Run(name, func(t *testing.T) {
			th := loadTheme(t, name)
			st := New(th)
			if !st.Link.GetUnderline() {
				t.Fatal("stile link senza sottolineatura")
			}
			ls := linkSlot(th)
			if st.Link.GetForeground() != adaptive(ls) {
				t.Error("colore del link diverso dallo slot scelto da linkSlot")
			}
			fD, fL := ls.Resolve()
			bD, bL := th.Bg.Resolve()
			if r := contrastRatio(lipgloss.Color(fD), lipgloss.Color(bD)); r < minContrast {
				t.Errorf("link dark: contrasto %.2f < %.1f", r, minContrast)
			}
			if r := contrastRatio(lipgloss.Color(fL), lipgloss.Color(bL)); r < minContrast {
				t.Errorf("link light: contrasto %.2f < %.1f", r, minContrast)
			}
		})
	}
}
