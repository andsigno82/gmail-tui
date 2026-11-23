package render

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/list"

	"github.com/andsigno82/gmail-tui/internal/msg/richtext"
)

// List rende un elenco puntato (list.Dash) o numerato da Start con
// charm.land/lipgloss/v2/list (Piano 15, Task 8, D15-04). Ogni elemento e un
// blocco: un paragrafo a capo sulla larghezza meno il marcatore (le righe a capo
// stanno sotto il testo, non sotto il marcatore). Un figlio KindList e un elenco
// annidato: va sotto l'elemento precedente, allineato al suo testo, con
// numerazione propria, e non conta come elemento del genitore. Start < 1 (valore
// zero del modello, <ol> senza attributo) parte da 1. Le zone dei link si leggono
// dalle righe finali (scanZones): restano giuste dopo marcatore e annidamento.
func List(b richtext.Block, c Ctx) Out {
	if c.Width <= 0 {
		return Out{}
	}

	// Marcatori: uno per elemento (non per elenco annidato); un elenco annidato
	// senza elemento prima prende un marcatore vuoto.
	start := max(b.Start, 1)
	var markers []string
	n := 0
	for i, ch := range b.Children {
		if ch.Kind == richtext.KindList {
			if i == 0 {
				markers = append(markers, "")
			}
			continue
		}
		switch {
		case b.Ordered:
			markers = append(markers, fmt.Sprintf("%d.", start+n))
		default:
			markers = append(markers, "-")
		}
		n++
	}
	if len(markers) == 0 {
		return Out{}
	}
	markerW := 0
	for _, m := range markers {
		markerW = max(markerW, lipgloss.Width(m))
	}
	markerW++ // spazio dopo il marcatore

	inner := Ctx{Width: max(1, c.Width-markerW), St: c.St, Depth: c.Depth}
	var bodies [][]string
	for i, ch := range b.Children {
		if ch.Kind == richtext.KindList {
			o := List(ch, inner)
			if i == 0 || len(bodies) == 0 {
				bodies = append(bodies, o.Lines)
			} else {
				bodies[len(bodies)-1] = append(bodies[len(bodies)-1], o.Lines...)
			}
			continue
		}
		bodies = append(bodies, renderChild(ch, inner, lipgloss.NewStyle()).Lines)
	}

	items := make([]any, len(bodies))
	for i, l := range bodies {
		items[i] = strings.Join(l, "\n")
	}
	indent := strings.Repeat(" ", markerW)
	l := list.New(items...).
		Enumerator(func(_ list.Items, i int) string { return markers[i] }).
		EnumeratorStyle(lipgloss.NewStyle().PaddingRight(1)).
		Indenter(func(list.Items, int) string { return indent }).
		IndenterStyle(lipgloss.NewStyle())
	lines := strings.Split(l.String(), "\n")
	return Out{Lines: lines, Zones: scanZones(lines)}
}
