package render

import (
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"

	"github.com/andsigno82/gmail-tui/internal/msg/richtext"
)

// Table rende una tabella con charm.land/lipgloss/v2/table (Piano 15, Task 8,
// D15-02, D15-T8). Le larghezze si calcolano qui: ogni colonna ha una larghezza
// minima (parola piu lunga; per l'intestazione il testo intero, perche lipgloss
// tronca l'intestazione a una riga) e una naturale (testo su una riga). Se la
// somma delle minime piu padding (1+1 per colonna) e bordi (colonne+1) supera
// c.Width, o la tabella supera MaxTableRows x MaxTableCols (D15-T4), ricade su
// testo ("Intestazione: valore" per riga). Altrimenti le celle si mandano a capo
// qui, sulla larghezza assegnata, e a lipgloss restano contenuti che gia
// entrano: Width(c.Width).FitContent() non li ricomprime. Le zone dei link si
// leggono dal testo reso (scanZones), a capo, bordi e colonne precedenti inclusi.
func Table(b richtext.Block, c Ctx) Out {
	nc := 0
	for _, r := range b.Rows {
		nc = max(nc, len(r))
	}
	if c.Width <= 0 || nc == 0 {
		return Out{}
	}
	hasHeader := allHeader(b.Rows[0])
	if len(b.Rows) > richtext.MaxTableRows || nc > richtext.MaxTableCols {
		return tableText(b, c, hasHeader)
	}

	cell := func(r, j int) richtext.Cell {
		if j < len(b.Rows[r]) {
			return b.Rows[r][j]
		}
		return richtext.Cell{}
	}
	natural := make([]int, nc)
	minw := make([]int, nc)
	for j := range nc {
		natural[j], minw[j] = 1, 1
	}
	for r := range b.Rows {
		for j := range nc {
			nat, mn := cellMetrics(cell(r, j).Runs)
			natural[j] = max(natural[j], nat)
			if r == 0 && hasHeader {
				mn = nat
			}
			minw[j] = max(minw[j], mn)
		}
	}
	budget := c.Width - (nc + 1) - 2*nc
	sumMin, sumNat := 0, 0
	for j := range nc {
		sumMin += minw[j]
		sumNat += natural[j]
	}
	if sumMin > budget {
		return tableText(b, c, hasHeader)
	}
	widths := natural
	if sumNat > budget {
		widths = shrink(minw, natural, budget)
	}

	cellStyle := func(r, j int) lipgloss.Style {
		if cell(r, j).Header {
			return c.St.TableHeader
		}
		return c.St.TableCell
	}
	render := func(r, j int) string {
		clr := cell(r, j)
		return strings.Join(wrapRuns(toRuns(clr.Runs), widths[j], cellStyle(r, j), c).Lines, "\n")
	}
	t := table.New().
		Width(c.Width).
		FitContent().
		BorderStyle(c.St.TableBorder).
		// Solo il padding: colori e grassetto (TableHeader/TableCell) sono gia
		// applicati ai tratti da wrapRuns, cosi non si sovrappongono agli stili
		// dei link dentro la cella.
		StyleFunc(func(int, int) lipgloss.Style { return lipgloss.NewStyle().Padding(0, 1) })
	first := 0
	if hasHeader {
		hdr := make([]string, nc)
		for j := range nc {
			hdr[j] = render(0, j)
		}
		t.Headers(hdr...)
		first = 1
	}
	for r := first; r < len(b.Rows); r++ {
		row := make([]string, nc)
		for j := range nc {
			row[j] = render(r, j)
		}
		t.Row(row...)
	}
	lines := strings.Split(t.Render(), "\n")
	return Out{Lines: lines, Zones: scanZones(lines)}
}

// allHeader dice se la riga e un'intestazione: non vuota e tutta di <th>.
func allHeader(r []richtext.Cell) bool {
	if len(r) == 0 {
		return false
	}
	for _, c := range r {
		if !c.Header {
			return false
		}
	}
	return true
}

// cellMetrics ritorna la larghezza naturale (testo su una riga, spazi collassati)
// e la minima (parola piu lunga) di una cella.
func cellMetrics(runs []richtext.Run) (natural, minw int) {
	for i, w := range tokenize(toRuns(runs)) {
		ww := w.width()
		natural += ww
		if i > 0 && w.gap {
			natural++
		}
		minw = max(minw, ww)
	}
	return natural, minw
}

// shrink distribuisce budget colonne tra le colonne: ognuna parte dalla minima e
// riceve parte dello spazio extra in proporzione a quanto la sua larghezza
// naturale supera la minima; il resto va una colonna alla volta alle piu larghe
// ancora sotto la naturale. Precondizione: somma(minw) <= budget < somma(natural).
func shrink(minw, natural []int, budget int) []int {
	w := append([]int(nil), minw...)
	extra := budget
	need, total := make([]int, len(w)), 0
	for j := range w {
		extra -= w[j]
		need[j] = natural[j] - minw[j]
		total += need[j]
	}
	if total == 0 {
		return w
	}
	given := 0
	for j := range w {
		add := extra * need[j] / total
		w[j] += add
		given += add
	}
	for left := extra - given; left > 0; left-- {
		best := -1
		for j := range w {
			if w[j] < natural[j] && (best < 0 || natural[j]-w[j] > natural[best]-w[best]) {
				best = j
			}
		}
		if best < 0 {
			break
		}
		w[best]++
	}
	return w
}

// tableText e la ricaduta su testo (D15-T8): per ogni riga del corpo, una riga di
// testo "Intestazione: valore" per cella, a capo sulla larghezza, righe separate
// da una riga vuota. Senza intestazione restano i soli valori; celle vuote
// saltate. Una tabella con la sola intestazione mostra i titoli.
func tableText(b richtext.Block, c Ctx, hasHeader bool) Out {
	var parts []Out
	var hdr []richtext.Cell
	body := b.Rows
	if hasHeader {
		hdr, body = b.Rows[0], b.Rows[1:]
	}
	label := func(j int) []irun {
		if j >= len(hdr) {
			return nil
		}
		l := toRuns(hdr[j].Runs)
		if len(strings.Fields(joinText(l))) == 0 {
			return nil
		}
		st := c.St.TableHeader
		for i := range l {
			l[i].st = &st
		}
		return append(l, irun{text: ": ", st: &st})
	}
	line := func(label, value []irun) {
		if len(strings.Fields(joinText(value))) == 0 {
			return
		}
		parts = append(parts, wrapRuns(append(label, value...), c.Width, c.St.TableCell, c))
	}
	if len(body) == 0 {
		for j := range hdr {
			line(nil, toRuns(hdr[j].Runs))
		}
	}
	blank := Out{Lines: []string{""}, Zones: [][]Zone{nil}}
	for ri, row := range body {
		if ri > 0 {
			parts = append(parts, blank)
		}
		for j, cl := range row {
			line(label(j), toRuns(cl.Runs))
		}
	}
	return Stack(parts...)
}

// joinText concatena il testo dei tratti (solo per decidere se una cella e vuota).
func joinText(rs []irun) string {
	var b strings.Builder
	for _, r := range rs {
		b.WriteString(r.text)
	}
	return b.String()
}
