// Package labelpick rende il corpo del pannello label su selezione (Piano 10, Task
// 7): il titolo, la riga del filtro e l'elenco delle label, ciascuna con la sua
// casella (☑, ☐, ◪). Colori solo dagli slot del tema (styles), mai fissi.
package labelpick

import (
	"strings"

	"github.com/andsigno82/gmail-tui/internal/ui/styles"
)

// Row è una riga del pannello: la casella e il nome della label.
type Row struct {
	Mark string
	Name string
}

// Render compone il pannello. La riga sotto il cursore usa gli slot di selezione;
// il pannello mostra al massimo maxRows righe, con la riga del cursore sempre visibile.
// Con nessuna riga mostra empty al posto dell'elenco.
func Render(st styles.Styles, title, filter string, rows []Row, cursor, maxRows int, empty string) string {
	var b strings.Builder
	// Piano 15, Task 16: con title vuoto il titolo e della finestra che incornicia il
	// pannello, non del corpo.
	if title != "" {
		b.WriteString(st.ThreadHeader.Render(title))
		b.WriteString("\n")
	}
	b.WriteString(filter)
	if len(rows) == 0 {
		b.WriteString("\n" + st.ListDate.Render(empty))
		return b.String()
	}
	maxRows = max(maxRows, 1)
	start := 0
	if cursor >= maxRows {
		start = cursor - maxRows + 1
	}
	end := min(start+maxRows, len(rows))
	for i := start; i < end; i++ {
		r := rows[i]
		b.WriteString("\n")
		if i == cursor {
			b.WriteString(st.ListCheckSel.Render(r.Mark) + st.ListSelected.Render(" "+r.Name))
		} else {
			b.WriteString(st.ListCheck.Render(r.Mark) + " " + st.ListNormal.Render(r.Name))
		}
	}
	return b.String()
}
