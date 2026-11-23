// Package render trasforma i blocchi di internal/msg/richtext in righe gia
// stilizzate con le loro zone cliccabili (Piano 15, Task 3, D15-T3). Questo file
// fissa il contratto: Ctx e le firme dei renderer; i corpi sono riempiti dai
// Task 6 (Link, Image), 7 (Button, Attachment) e 8 (Table, Quote, List). I
// renderer usano solo slot del tema tramite styles.Styles e non fanno mai
// richieste di rete.
package render

import (
	"strings"

	"github.com/andsigno82/gmail-tui/internal/msg/richtext"
	"github.com/andsigno82/gmail-tui/internal/ui/styles"
)

// Ctx e il contesto di rendering di un blocco.
type Ctx struct {
	Width int           // larghezza disponibile in colonne
	St    styles.Styles // stili derivati dal tema
	Depth int           // livello di annidamento (citazioni, elenchi)
}

// Dispatch rende un blocco scegliendo il renderer per Kind (Piano 15, Task 3).
// KindRule e reso qui (una linea di Width colonne in St.Divider, nessuna zona);
// KindPre usa Paragraph (a capo gia conservati dal modello). Un Kind sconosciuto
// da un Out vuoto.
func Dispatch(b richtext.Block, c Ctx) Out {
	switch b.Kind {
	case richtext.KindParagraph, richtext.KindPre:
		return Paragraph(b, c)
	case richtext.KindQuote:
		return Quote(b, c)
	case richtext.KindList:
		return List(b, c)
	case richtext.KindTable:
		return Table(b, c)
	case richtext.KindButton:
		return Button(b, c)
	case richtext.KindImage:
		return Image(b.Alt, c)
	case richtext.KindRule:
		if c.Width <= 0 {
			return Out{}
		}
		return Out{
			Lines: []string{c.St.Divider.Render(strings.Repeat("─", c.Width))},
			Zones: [][]Zone{nil},
		}
	}
	return Out{}
}

// Paragraph rende i Runs di un paragrafo a capo sulla larghezza (Task 6/8).
func Paragraph(b richtext.Block, c Ctx) Out { return Out{} }

// Link rende un link con controllo di dominio (D15-07, Task 6).
func Link(text, href string, c Ctx) Out { return Out{} }

// Image rende un'immagine come chip con il testo alternativo (D15-08, Task 6).
func Image(alt string, c Ctx) Out { return Out{} }

// Button e Attachment sono in button.go e attachment.go (Piano 15, Task 7).

// Table rende una tabella, con ricaduta su testo se troppo larga (D15-T8, Task 8).
func Table(b richtext.Block, c Ctx) Out { return Out{} }

// Quote rende una citazione con barra colorata, annidata via Indent (D15-03, Task 8).
func Quote(b richtext.Block, c Ctx) Out { return Out{} }

// List rende un elenco ordinato o puntato (D15-04, Task 8).
func List(b richtext.Block, c Ctx) Out { return Out{} }
