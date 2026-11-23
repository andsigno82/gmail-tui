package render

// Immagini come chip testuale (Piano 15, Task 6, D15-08): nessuna richiesta di
// rete, nessuna emoji (Q15-4), nessuna zona cliccabile. L'alt e non fidato:
// ripulito come il testo dei link.

import (
	"charm.land/lipgloss/v2"
)

// maxAltCols e la larghezza massima del testo alternativo in colonne.
const maxAltCols = 80

// TruncCols è in button.go (versione che gestisce anche larghezza 0).

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
		text = "[img: " + TruncCols(alt, max) + "]"
	}
	return Out{Lines: []string{c.St.ChipImage.Render(text)}, Zones: [][]Zone{nil}}
}
