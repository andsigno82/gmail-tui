package render

// Immagini come chip testuale (Piano 15, Task 6, D15-08): nessuna richiesta di
// rete, nessuna emoji (Q15-4), nessuna zona cliccabile. L'alt e non fidato:
// ripulito come il testo dei link.

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// maxAltCols e la larghezza massima del testo alternativo in colonne.
const maxAltCols = 80

// truncCols tronca s a max colonne con "…" finale se non ci sta.
func truncCols(s string, max int) string {
	if lipgloss.Width(s) <= max {
		return s
	}
	var b strings.Builder
	w := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if w+rw > max-1 {
			break
		}
		b.WriteRune(r)
		w += rw
	}
	b.WriteString("…")
	return b.String()
}

// Image rende un'immagine come chip "[img: alt]" ("[img]" con alt vuoto). L'alt
// e troncato a 80 colonne e comunque entro c.Width. Nessuna zona.
func Image(alt string, c Ctx) Out {
	text := "[img]"
	if alt = cleanText(alt); alt != "" {
		max := maxAltCols
		if c.Width > 0 {
			if avail := c.Width - lipgloss.Width("[img: ]"); avail < max {
				max = avail
			}
		}
		if max < 1 {
			max = 1
		}
		text = "[img: " + truncCols(alt, max) + "]"
	}
	return Out{Lines: []string{c.St.ChipImage.Render(text)}, Zones: [][]Zone{nil}}
}
