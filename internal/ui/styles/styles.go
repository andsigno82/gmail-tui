// Package styles traduce gli slot semantici di internal/theme in stili
// lipgloss v2 per la TUI. Ogni colore deriva ESCLUSIVAMENTE da uno slot del
// tema: mai hex diretti. Ricostruire con New se cambia il tema.
package styles

import (
	"image/color"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"

	"github.com/andsigno82/gmail-tui/internal/theme"
)

// Styles raccoglie gli stili della TUI per il tema corrente.
type Styles struct {
	ListNormal      lipgloss.Style // riga normale della lista messaggi
	ListUnread      lipgloss.Style // riga non letta (marcatore ● e subject)
	ListStarred     lipgloss.Style // riga con stella (★)
	ListDate        lipgloss.Style // metadati riga: data relativa, mittente
	ListSelected    lipgloss.Style // riga sotto il cursore
	SearchInput     lipgloss.Style // prompt/cursore della ricerca
	StatusOK        lipgloss.Style // status bar: feedback positivo
	StatusErr       lipgloss.Style // status bar: feedback errore
	StatusBar       lipgloss.Style // status bar: stato normale
	HelpView        lipgloss.Style // barra help contestuale / overlay
	ThreadHeader    lipgloss.Style // intestazione del thread
	ThreadHeaderKey lipgloss.Style // etichette dell'header (Da:/A:/Oggetto:)
	Quoted          lipgloss.Style // righe citate attenuate
	Border          lipgloss.Style // riquadri (viewport, overlay)
	Spinner         lipgloss.Style // spinner durante il caricamento
	DemoBanner      lipgloss.Style // banner "demo mode" senza credenziali
}

// New costruisce gli stili per il tema dato. La variante dark/light di ogni
// slot è risolta adattivamente al rendering dal background del terminale.
func New(t theme.Theme) Styles {
	return Styles{
		ListNormal:      lipgloss.NewStyle().Foreground(adaptive(t.Fg)),
		ListUnread:      lipgloss.NewStyle().Foreground(adaptive(t.Unread)).Bold(true),
		ListStarred:     lipgloss.NewStyle().Foreground(adaptive(t.Starred)),
		ListDate:        lipgloss.NewStyle().Foreground(adaptive(t.Muted)),
		ListSelected:    lipgloss.NewStyle().Foreground(adaptive(t.Fg)).Background(adaptive(t.Selection)).Bold(true),
		SearchInput:     lipgloss.NewStyle().Foreground(adaptive(t.Accent)),
		StatusOK:        lipgloss.NewStyle().Foreground(adaptive(t.Fg)).Background(adaptive(t.Success)).Padding(0, 1),
		StatusErr:       lipgloss.NewStyle().Foreground(adaptive(t.Fg)).Background(adaptive(t.Error)).Bold(true).Padding(0, 1),
		StatusBar:       lipgloss.NewStyle().Foreground(adaptive(t.Fg)).Background(adaptive(t.Primary)).Padding(0, 1),
		HelpView:        lipgloss.NewStyle().Foreground(adaptive(t.Help)),
		ThreadHeader:    lipgloss.NewStyle().Foreground(adaptive(t.Primary)).Bold(true).Padding(0, 1),
		ThreadHeaderKey: lipgloss.NewStyle().Foreground(adaptive(t.Muted)),
		Quoted:          lipgloss.NewStyle().Foreground(adaptive(t.Quoted)).Italic(true),
		Border:          lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(adaptive(t.Border)),
		Spinner:         lipgloss.NewStyle().Foreground(adaptive(t.Accent)).Bold(true),
		DemoBanner:      lipgloss.NewStyle().Foreground(adaptive(t.Bg)).Background(adaptive(t.Warning)).Bold(true).Padding(0, 1),
	}
}

// adaptive converte uno Slot in un colore lipgloss: la coppia dark/light
// dello slot diventa adattiva. Nota v2: lipgloss.AdaptiveColor è stato
// rimosso; compat.AdaptiveColor (UPGRADE_GUIDE_V2.md) è il sostituto drop-in
// ufficiale con le stesse semantiche, e risolve la variante al rendering.
// In Bubble Tea il background arriva da tea.BackgroundColorMsg; compat verrà
// aggiornato in main (Task 8/9) al primo messaggio ricevuto.
func adaptive(s theme.Slot) color.Color {
	dark, light := s.Resolve()
	return compat.AdaptiveColor{
		Dark:  lipgloss.Color(dark),
		Light: lipgloss.Color(light),
	}
}
