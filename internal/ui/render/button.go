package render

import (
	"fmt"
	"hash/fnv"
	"net/url"
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"

	"github.com/andsigno82/gmail-tui/internal/msg/richtext"
)

// maxURLLen e la lunghezza massima (in byte) di un URL cliccabile (D15-T13).
const maxURLLen = 2048

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
	label := cleanLabel(runsText(b.Runs))
	if label == "" {
		label = "(senza testo)"
	}
	u, ok := safeURL(runsHref(b.Runs))

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
		rows = append(rows, rb.Left+" "+padCols(truncCols(l, inner), inner)+" "+rb.Right)
	}
	rows = append(rows, rb.BottomLeft+strings.Repeat(rb.Bottom, inner+2)+rb.BottomRight)

	id := zoneID(u)
	host := cleanLabel(b.Host)
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

// isInvisible riconosce i caratteri bidi e zero-width che nascondono il testo
// reale (D15-T13): U+202A-U+202E, U+2066-U+2069, U+200E, U+200F, U+200B-U+200D, U+FEFF.
func isInvisible(r rune) bool {
	switch {
	case r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069:
		return true
	case r == 0x200E || r == 0x200F, r >= 0x200B && r <= 0x200D, r == 0xFEFF:
		return true
	}
	return false
}

// cleanLabel sanifica un testo non fidato per la riga (Piano 15, Task 7, D15-T13):
// toglie bidi, zero-width e controlli, riduce gli spazi (a capo e tab inclusi) a
// uno solo. Ripete in locale la regola del Task 5 (richtext non esporta sanitizeText).
func cleanLabel(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case isInvisible(r):
		case unicode.IsSpace(r):
			b.WriteRune(' ')
		case unicode.IsControl(r):
		default:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// safeURL ripulisce raw (controlli, spazi, bidi, zero-width) e accetta solo http,
// https con host e mailto con indirizzo, entro maxURLLen byte (D15-T13). Altrimenti
// false: il testo resta senza zona.
func safeURL(raw string) (string, bool) {
	s := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.IsSpace(r) || isInvisible(r) {
			return -1
		}
		return r
	}, raw)
	if s == "" || len(s) > maxURLLen {
		return "", false
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return s, u.Host != ""
	case "mailto":
		return s, u.Opaque != ""
	}
	return "", false
}

// truncCols tronca s a w colonne (lipgloss.Width), chiudendo con "…" se serve.
func truncCols(s string, w int) string {
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
