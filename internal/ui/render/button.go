package render

import (
	"fmt"
	"hash/fnv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/andsigno82/gmail-tui/internal/msg/richtext"
)

// Button rende un bottone a riquadro con bordo arrotondato, padding orizzontale
// 1 e l'host di destinazione accanto (Piano 15, Task 7, D15-01). Ogni riga del
// riquadro e una Zone ZoneButton con l'URL e lo stesso id OSC 8. Se il link ha
// Warn, il riquadro ha una riga di avviso con ⚠. Con c.Width troppo stretto per
// il riquadro (o <= 0) Out vuoto. L'host compare solo se entra intero.
func Button(b richtext.Block, c Ctx) Out {
	avail := c.Width - 4 // bordo (2) + padding orizzontale (1 + 1)
	if avail < 1 {
		return Out{}
	}
	label := cleanText(runsText(b.Runs))
	if label == "" {
		label = "(senza testo)"
	}
	u, _, ok := safeHref(runsHref(b.Runs)) // stessa sanificazione di Link (Piano 15, Task 14, lavoro B)

	lines := []string{label}
	if b.Warn {
		lines = append(lines, "⚠ dominio diverso dal testo")
	}
	inner := 0
	for _, l := range lines {
		inner = max(inner, lipgloss.Width(l))
	}
	inner = min(inner, avail)
	boxW := inner + 4

	rb := lipgloss.RoundedBorder()
	rows := []string{rb.TopLeft + strings.Repeat(rb.Top, inner+2) + rb.TopRight}
	for _, l := range lines {
		rows = append(rows, rb.Left+" "+padCols(TruncCols(l, inner), inner)+" "+rb.Right)
	}
	rows = append(rows, rb.BottomLeft+strings.Repeat(rb.Bottom, inner+2)+rb.BottomRight)

	id := zoneID(u)
	host := cleanText(b.Host)
	hostSeg := ""
	if room := c.Width - boxW - 1; host != "" && lipgloss.Width("→ "+host) <= room {
		hostSeg = " " + c.St.ButtonHost.Render("→ "+host)
	}

	var o Out
	for i, row := range rows {
		line := c.St.Button.Render(row)
		if ok {
			line = osc8Open(u, id) + line + osc8Close
		}
		if i == 1 {
			line += hostSeg
		}
		o.Lines = append(o.Lines, line)
		var zs []Zone
		if ok {
			zs = []Zone{{X0: 0, X1: boxW, Kind: ZoneButton, URL: u, ID: id, Warn: b.Warn}}
			if b.Warn {
				zs[0].Shown = label // Piano 15, Task 16: per la conferma dell'app
			}
		}
		o.Zones = append(o.Zones, zs)
	}
	return o
}

// runsText concatena il testo dei Runs.
func runsText(rs []richtext.Run) string {
	var b strings.Builder
	for _, r := range rs {
		b.WriteString(r.Text)
	}
	return b.String()
}

// runsHref ritorna il primo href non vuoto dei Runs, o "".
func runsHref(rs []richtext.Run) string {
	for _, r := range rs {
		if r.Href != "" {
			return r.Href
		}
	}
	return ""
}

// TruncCols tronca s a w colonne (lipgloss.Width), chiudendo con "…" se serve.
func TruncCols(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	var b strings.Builder
	acc := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if acc+rw > w-1 {
			break
		}
		b.WriteRune(r)
		acc += rw
	}
	return b.String() + "…"
}

// padCols aggiunge spazi a destra fino a w colonne.
func padCols(s string, w int) string {
	return s + strings.Repeat(" ", max(0, w-lipgloss.Width(s)))
}

// zoneID ricava l'id OSC 8 comune alle righe di un elemento dal suo URL.
func zoneID(u string) string {
	h := fnv.New32a()
	h.Write([]byte(u))
	return fmt.Sprintf("gmb%08x", h.Sum32())
}

// osc8Open e osc8Close sono in link.go (Task 6).
