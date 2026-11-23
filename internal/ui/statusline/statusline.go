// Package statusline compone la barra di stato a segmenti (Piano 15, Task 13,
// D15-12): modo, cartella, stato e help, con cambio di sfondo tra segmenti.
package statusline

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/andsigno82/gmail-tui/internal/ui/styles"
)

// SegKind è il ruolo di un segmento della barra di stato (Piano 15, Task 13).
type SegKind int

const (
	SegMode   SegKind = iota // modo corrente (es. NORMALE, RICERCA)
	SegFolder                // cartella aperta
	SegStatus                // messaggio di stato
	SegHelp                  // suggerimenti di tasti: primo segmento a essere troncato
)

// Segment è un pezzo di testo della barra di stato (Piano 15, Task 13).
type Segment struct {
	Text string
	Kind SegKind
}

// Line restituisce la barra di stato larga esattamente width celle (Piano 15, Task 13).
// I segmenti vuoti sono saltati. Lo sfondo cambia tra segmenti adiacenti: con due o
// più stili non serve alcun separatore (nessun glifo, D15-T12); con un solo stile si
// usa uno spazio. Se la somma supera width si tronca prima il segmento help, poi gli
// altri a partire dall'ultimo. Il residuo a destra è riempito con spazi senza stile.
func Line(segs []Segment, width int, st styles.Styles) string {
	if width <= 0 {
		return ""
	}
	sep := sepWidth(st)
	kept := make([]Segment, 0, len(segs))
	for _, s := range segs {
		if s.Text != "" {
			kept = append(kept, s)
		}
	}
	kept = tronca(kept, width, sep)

	var b strings.Builder
	for i, s := range kept {
		if i > 0 && sep > 0 {
			b.WriteString(st.StatusSep.Render(" "))
		}
		b.WriteString(stileSegmento(st, i).Render(" " + s.Text + " "))
	}
	riga := b.String()
	if w := lipgloss.Width(riga); w < width {
		riga += strings.Repeat(" ", width-w)
	}
	return riga
}

// sepWidth è il costo del separatore tra segmenti adiacenti: 0 quando gli stili
// alternano lo sfondo (almeno due), 1 spazio altrimenti.
func sepWidth(st styles.Styles) int {
	if len(st.StatusSegment) >= 2 {
		return 0
	}
	return 1
}

// stileSegmento sceglie lo stile del segmento i: lo sfondo alterna sugli stili
// disponibili, così due segmenti adiacenti hanno sfondi diversi.
func stileSegmento(st styles.Styles, i int) lipgloss.Style {
	if len(st.StatusSegment) == 0 {
		return lipgloss.NewStyle()
	}
	return st.StatusSegment[i%len(st.StatusSegment)]
}

// larghezzaSegmenti è la larghezza occupata da segs: testo, un margine di una cella
// per lato e i separatori tra segmenti.
func larghezzaSegmenti(segs []Segment, sep int) int {
	w := 0
	for _, s := range segs {
		w += lipgloss.Width(s.Text) + 2
	}
	if len(segs) > 1 {
		w += sep * (len(segs) - 1)
	}
	return w
}

// tronca riduce segs fino a farli stare in width: prima il testo dei segmenti help
// (dall'ultimo), poi quello degli altri a partire dall'ultimo. Un segmento il cui
// testo scende a zero celle è rimosso.
func tronca(segs []Segment, width, sep int) []Segment {
	for larghezzaSegmenti(segs, sep) > width {
		i := ultimoHelp(segs)
		if i < 0 {
			break
		}
		segs = riduci(segs, i, larghezzaSegmenti(segs, sep)-width)
	}
	for larghezzaSegmenti(segs, sep) > width && len(segs) > 0 {
		segs = riduci(segs, len(segs)-1, larghezzaSegmenti(segs, sep)-width)
	}
	return segs
}

// ultimoHelp è l'indice dell'ultimo segmento SegHelp, -1 se manca.
func ultimoHelp(segs []Segment) int {
	for i := len(segs) - 1; i >= 0; i-- {
		if segs[i].Kind == SegHelp {
			return i
		}
	}
	return -1
}

// riduci toglie eccesso celle al testo del segmento i; se non resta nulla rimuove il
// segmento.
func riduci(segs []Segment, i, eccesso int) []Segment {
	n := lipgloss.Width(segs[i].Text) - eccesso
	if n <= 0 {
		return append(segs[:i], segs[i+1:]...)
	}
	segs[i].Text = taglia(segs[i].Text, n)
	return segs
}

// taglia restituisce il prefisso di s largo al più n celle (per rune).
func taglia(s string, n int) string {
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r)) > n {
		r = r[:len(r)-1]
	}
	return string(r)
}
