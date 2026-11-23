package render

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/andsigno82/gmail-tui/internal/msg/richtext"
	"github.com/andsigno82/gmail-tui/internal/ui/styles"
)

// quoteBar e la barra verticale della citazione, seguita da uno spazio
// (Piano 15, Task 8, D15-03): occupa quoteBarW colonne per livello.
const (
	quoteBar  = "▎"
	quoteBarW = 2
)

// Quote rende una citazione con barra laterale colorata e livelli annidati
// (Piano 15, Task 8, D15-03). Il livello di una barra e assoluto: la barra i-esima
// di una riga usa St.QuoteBar[i % len], a cicli. Una citazione ha max(1,
// b.Depth-c.Depth) barre proprie, dove c.Depth conta le barre gia disegnate dalle
// citazioni che la contengono: cosi sia un blockquote annidato (Depth 0 poi 1, o
// 1 poi 2) sia una citazione piatta di FromText (">>>" = Depth 3, tre barre)
// producono il numero giusto di barre, qualunque sia la convenzione del parser.
// I figli si rendono sulla larghezza meno le barre e si spostano con Indent, cosi
// le zone dei link restano allineate alle colonne reali.
func Quote(b richtext.Block, c Ctx) Out {
	if c.Width <= 0 {
		return Out{}
	}
	k := min(max(1, b.Depth-c.Depth), richtext.MaxDepth)
	if maxK := max(1, (c.Width-1)/quoteBarW); k > maxK {
		k = maxK
	}
	inner := Ctx{Width: max(1, c.Width-quoteBarW*k), St: c.St, Depth: c.Depth + k}
	var parts []Out
	for _, ch := range b.Children {
		o := renderChild(ch, inner, c.St.Quoted)
		if len(o.Lines) == 0 {
			continue
		}
		if len(parts) > 0 {
			parts = append(parts, Out{Lines: []string{""}, Zones: [][]Zone{nil}})
		}
		parts = append(parts, o)
	}
	if len(parts) == 0 {
		return Out{}
	}
	var bars strings.Builder
	for i := range k {
		bars.WriteString(barStyle(c.St, c.Depth+i).Render(quoteBar) + " ")
	}
	o := Indent(Stack(parts...), quoteBarW*k)
	for i, l := range o.Lines {
		o.Lines[i] = bars.String() + strings.TrimPrefix(l, strings.Repeat(" ", quoteBarW*k))
	}
	return o
}

// barStyle e lo stile della barra al livello assoluto level (QuoteBar a cicli).
func barStyle(st styles.Styles, level int) lipgloss.Style {
	if len(st.QuoteBar) == 0 {
		return lipgloss.NewStyle()
	}
	return st.QuoteBar[level%len(st.QuoteBar)]
}

// renderChild rende un blocco figlio di citazione o elenco (Piano 15, Task 8):
// paragrafi e testo preformattato con base come stile del testo semplice (stesso
// motore di Paragraph, Piano 15, Task 14), ogni altro Kind passa da Dispatch.
func renderChild(b richtext.Block, c Ctx, base lipgloss.Style) Out {
	switch b.Kind {
	case richtext.KindParagraph, richtext.KindPre:
		return paragraphWith(b, c, base)
	}
	return Dispatch(b, c)
}
