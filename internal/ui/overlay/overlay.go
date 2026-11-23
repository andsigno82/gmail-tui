// Package overlay disegna finestre sovrapposte con ombra sopra una vista già
// renderizzata (Piano 15, Task 12, D15-11). Usa il compositor nativo di lipgloss
// v2: uno Layer con celle di spazio stilizzate copre le celle sottostanti (spike
// verificato nei test), quindi l'ombra è un layer di retino ░ (stile classico
// TUI, decisione di Andrea 2026-11-23) nel colore dello slot border, spostato di
// (+2, +1) sotto la finestra. Le celle del terminale non hanno alpha: nessuna
// trasparenza vera (D15-T11). Nessun colore fisso: tutto deriva da styles.Styles.
package overlay

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/andsigno82/gmail-tui/internal/ui/styles"
)

// Spostamento dell'ombra rispetto alla finestra (D15-T11), carattere del retino e
// dimensioni minime (Piano 15, Task 12).
const (
	shadowDX    = 2
	shadowDY    = 1
	shadowRune  = "░"
	minW, minH  = 3, 3 // bordo + almeno una cella interna
	paddingMinW = 5    // sotto questa larghezza interna niente padding orizzontale
)

// Window rende il contenuto in un riquadro di w x h celle esterne con bordo
// arrotondato sfumato primary->accent (D15-13). Se title non è vuoto occupa la
// prima riga interna, con la stessa sfumatura sulle lettere (mai sul contenuto).
// Contenuto troppo lungo o largo è tagliato: la finestra non cambia dimensione.
// Sotto 3x3 si arriva a 3x3 (Piano 15, Task 12).
func Window(content, title string, st styles.Styles, w, h int) string {
	w, h = max(w, minW), max(h, minH)
	innerW, innerH := w-2, h-2
	pad := 0
	if innerW >= paddingMinW {
		pad = 1
	}
	cw := innerW - 2*pad

	primary, accent := st.OverlayBorder.GetForeground(), st.Spinner.GetForeground()

	var lines []string
	if title != "" && innerH >= 2 {
		lines = append(lines, gradientText(clip(title, cw), primary, accent))
		innerH--
	}
	for i, l := range strings.Split(content, "\n") {
		if i >= innerH {
			break
		}
		lines = append(lines, clip(l, cw))
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForegroundBlend(primary, accent, primary).
		Padding(0, pad).
		Width(w).Height(h).
		Render(strings.Join(lines, "\n"))
}

// Overlay sovrappone win a base con il compositor di lipgloss: base Z=0, ombra
// Z=1 in (x+2, y+1), finestra Z=2 in (x, y). Le coordinate sono riportate dentro
// la base in modo che finestra e ombra restino visibili; l'uscita ha sempre le
// dimensioni di base (finestra più grande: tagliata). Piano 15, Task 12.
//
// Il compositor lavora solo sulla banda di righe coperta da finestra e ombra (le
// righe sopra e sotto restano quelle della base): ogni riga e indipendente, quindi
// l'uscita e identica a quella sull'intera base (TestOverlayABandaEquivaleAlCompositorePieno)
// ma il buffer di celle e alto quanto la finestra e non quanto lo schermo (indagine
// prestazioni dopo il Piano 15: il buffer pieno era il costo di ogni frame con una
// finestra aperta).
func Overlay(base, win string, x, y int, st styles.Styles) string {
	lines := strings.Split(base, "\n")
	bw, bh := lipgloss.Width(base), len(lines)
	ww, wh := lipgloss.Width(win), lipgloss.Height(win)
	x = min(max(x, 0), max(bw-ww-shadowDX, 0))
	y = min(max(y, 0), max(bh-wh-shadowDY, 0))

	top := min(y, bh)
	bot := min(bh, y+wh+shadowDY) // ultima riga (esclusa) coperta dall'ombra
	band := strings.Join(lines[top:bot], "\n")
	bandH := bot - top

	row := strings.Repeat(shadowRune, ww)
	shadow := st.OverlayShadow.UnsetBackground().Render(strings.TrimSuffix(strings.Repeat(row+"\n", wh), "\n"))
	out := lipgloss.NewCompositor(
		lipgloss.NewLayer(band),
		lipgloss.NewLayer(shadow).X(x+shadowDX).Y(shadowDY).Z(1),
		lipgloss.NewLayer(win).X(x).Z(2),
	).Render()

	// Il render dei layer toglie le righe vuote finali: si riportano all'altezza
	// della banda, poi si taglia ciò che sfora (finestra più grande della base).
	if n := strings.Count(out, "\n") + 1; n < bandH {
		out += strings.Repeat("\n", bandH-n)
	}
	out = lipgloss.NewStyle().MaxWidth(bw).MaxHeight(bandH).Render(out)

	parts := make([]string, 0, 3)
	if top > 0 {
		parts = append(parts, strings.Join(lines[:top], "\n"))
	}
	parts = append(parts, out)
	if bot < bh {
		parts = append(parts, strings.Join(lines[bot:], "\n"))
	}
	return strings.Join(parts, "\n")
}

// Center dà le coordinate (x, y) che centrano win in base, mai negative.
func Center(base, win string) (x, y int) {
	x = (lipgloss.Width(base) - lipgloss.Width(win)) / 2
	y = (lipgloss.Height(base) - lipgloss.Height(win)) / 2
	return max(x, 0), max(y, 0)
}

// gradientText colora ogni lettera lungo la sfumatura a->b, in grassetto.
func gradientText(s string, a, b color.Color) string {
	runes := []rune(s)
	cols := lipgloss.Blend1D(len(runes), a, b)
	var sb strings.Builder
	for i, r := range runes {
		sb.WriteString(lipgloss.NewStyle().Foreground(cols[i]).Bold(true).Render(string(r)))
	}
	return sb.String()
}

// clip taglia una riga a n celle (con stili ANSI preservati).
func clip(s string, n int) string {
	return lipgloss.NewStyle().MaxWidth(n).Render(s)
}
