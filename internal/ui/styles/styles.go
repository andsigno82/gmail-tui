// Package styles traduce gli slot semantici di internal/theme in stili
// lipgloss v2 per la TUI. Ogni colore deriva ESCLUSIVAMENTE da uno slot del
// tema: mai hex diretti. Ricostruire con New se cambia il tema.
package styles

import (
	"image/color"
	"math"

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
	StatusErr       lipgloss.Style // barra colorata: conferme modali (invio, scarta, cestino)
	StatusBar       lipgloss.Style // status bar: stato normale
	HelpView        lipgloss.Style // barra help contestuale / overlay
	ThreadHeader    lipgloss.Style // intestazione del thread
	ThreadHeaderKey lipgloss.Style // etichette dell'header (Da:/A:/Oggetto:)
	Quoted          lipgloss.Style // righe citate attenuate
	Border          lipgloss.Style // riquadri (viewport, overlay)
	Spinner         lipgloss.Style // spinner durante il caricamento
	Divider         lipgloss.Style // separatore lista/anteprima a due colonne (Piano 7.2, Task 4)
	DividerFocus    lipgloss.Style // separatore con il focus sull'anteprima (slot primary)
	ToastInfo       lipgloss.Style // toast informativo: bordo accent (Piano 7.3, Task 1)
	ToastSuccess    lipgloss.Style // toast di esito positivo: bordo success
	ToastWarning    lipgloss.Style // toast d'avviso: bordo warning
	ToastError      lipgloss.Style // toast d'errore: bordo error (resta finché un tasto)
}

// New costruisce gli stili per il tema dato. La variante dark/light di ogni
// slot è risolta adattivamente al rendering dal background del terminale.
func New(t theme.Theme) Styles {
	return Styles{
		ListNormal:      lipgloss.NewStyle().Foreground(adaptive(t.Fg)),
		ListUnread:      lipgloss.NewStyle().Foreground(adaptive(t.Unread)).Bold(true),
		ListStarred:     lipgloss.NewStyle().Foreground(adaptive(t.Starred)),
		ListDate:        lipgloss.NewStyle().Foreground(adaptive(t.Muted)),
		ListSelected:    lipgloss.NewStyle().Foreground(onColor(t.Selection, t.Fg, t.Bg)).Background(adaptive(t.Selection)).Bold(true),
		SearchInput:     lipgloss.NewStyle().Foreground(adaptive(t.Accent)),
		StatusErr:       lipgloss.NewStyle().Foreground(onColor(t.Error, t.Fg, t.Bg)).Background(adaptive(t.Error)).Bold(true).Padding(0, 1),
		StatusBar:       lipgloss.NewStyle().Foreground(onColor(t.Primary, t.Fg, t.Bg)).Background(adaptive(t.Primary)).Padding(0, 1),
		HelpView:        lipgloss.NewStyle().Foreground(adaptive(t.Help)),
		ThreadHeader:    lipgloss.NewStyle().Foreground(adaptive(t.Primary)).Bold(true).Padding(0, 1),
		ThreadHeaderKey: lipgloss.NewStyle().Foreground(adaptive(t.Muted)),
		Quoted:          lipgloss.NewStyle().Foreground(adaptive(t.Quoted)).Italic(true),
		Border:          lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(adaptive(t.Border)),
		Spinner:         lipgloss.NewStyle().Foreground(adaptive(t.Accent)).Bold(true),
		Divider:         lipgloss.NewStyle().Foreground(adaptive(t.Border)),
		DividerFocus:    lipgloss.NewStyle().Foreground(adaptive(t.Primary)),
		ToastInfo:       toastStyle(t, t.Accent),
		ToastSuccess:    toastStyle(t, t.Success),
		ToastWarning:    toastStyle(t, t.Warning),
		ToastError:      toastStyle(t, t.Error),
	}
}

// toastStyle è il riquadro di una notifica (Piano 7.3, Task 1): superficie
// dello slot bg, bordo arrotondato nello slot edge del tipo. Il testo usa
// onColor sulla superficie, così vale il test di contrasto WCAG.
func toastStyle(t theme.Theme, edge theme.Slot) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(onColor(t.Bg, t.Fg, t.Accent)).
		Background(adaptive(t.Bg)).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(adaptive(edge)).
		BorderBackground(adaptive(t.Bg)).
		Padding(0, 1)
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

// minContrast è la soglia WCAG AA per il testo normale. Piano 7.2, Task 1.
const minContrast = 4.5

// onColor calcola il colore del testo su una barra colorata con sfondo bg.
// Piano 7.2, Task 1 (decisione U1): per la variante dark e light separatamente
// sceglie il primo tra fg e base (lo slot bg del tema) con contrasto WCAG >=
// 4.5 rispetto a bg; altrimenti nero o bianco, quello con contrasto maggiore.
func onColor(bg, fg, base theme.Slot) color.Color {
	bgD, bgL := bg.Resolve()
	fgD, fgL := fg.Resolve()
	baseD, baseL := base.Resolve()
	return compat.AdaptiveColor{
		Dark:  textOn(lipgloss.Color(bgD), lipgloss.Color(fgD), lipgloss.Color(baseD)),
		Light: textOn(lipgloss.Color(bgL), lipgloss.Color(fgL), lipgloss.Color(baseL)),
	}
}

// textOn applica la regola di onColor a un singolo lato (dark o light).
func textOn(bg, fg, base color.Color) color.Color {
	if contrastRatio(fg, bg) >= minContrast {
		return fg
	}
	if contrastRatio(base, bg) >= minContrast {
		return base
	}
	black, white := lipgloss.Color("#000000"), lipgloss.Color("#ffffff")
	if contrastRatio(black, bg) >= contrastRatio(white, bg) {
		return black
	}
	return white
}

// contrastRatio è il rapporto di contrasto WCAG 2.x tra due colori (da 1 a 21).
func contrastRatio(a, b color.Color) float64 {
	la, lb := relativeLuminance(a), relativeLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// relativeLuminance è la luminanza relativa WCAG 2.x (canali linearizzati).
func relativeLuminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA() // canali a 16 bit
	return 0.2126*linearChannel(r) + 0.7152*linearChannel(g) + 0.0722*linearChannel(b)
}

func linearChannel(v uint32) float64 {
	c := float64(v) / 65535
	if c <= 0.03928 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}
