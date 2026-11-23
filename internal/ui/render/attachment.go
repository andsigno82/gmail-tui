package render

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// attachNameCols e il limite di colonne del nome nel chip (Piano 15, Task 7).
const attachNameCols = 40

// Attachment rende il chip di un allegato "▣ nome · tipo · dimensione" in una riga
// (Piano 15, Task 7, D15-06). Il nome e sanificato e troncato a 40 colonne; il chip
// intero si tronca a c.Width se positivo. La Zone ZoneAttachment porta Index, senza URL.
func Attachment(name, ctype string, size int64, index int, c Ctx) Out {
	nm := cleanText(name)
	if nm == "" {
		nm = "(senza nome)"
	}
	nm = truncCols(nm, attachNameCols)
	tp := cleanText(ctype)
	if tp == "" {
		tp = "tipo sconosciuto"
	}
	chip := "▣ " + nm + " · " + tp + " · " + humanSize(size)
	if c.Width > 0 {
		chip = truncCols(chip, c.Width)
	}
	return Out{
		Lines: []string{c.St.ChipAttach.Render(chip)},
		Zones: [][]Zone{{{X0: 0, X1: lipgloss.Width(chip), Kind: ZoneAttachment, Index: index}}},
	}
}

// humanSize formatta una dimensione in byte: "N B", "N,N KB" o "N,N MB" (virgola,
// come nell'interfaccia in italiano). Valori negativi contano come 0.
func humanSize(n int64) string {
	switch {
	case n < 0:
		return "0 B"
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1<<20:
		return strings.Replace(fmt.Sprintf("%.1f KB", float64(n)/1024), ".", ",", 1)
	default:
		return strings.Replace(fmt.Sprintf("%.1f MB", float64(n)/(1<<20)), ".", ",", 1)
	}
}
