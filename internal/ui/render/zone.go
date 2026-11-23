package render

import "strings"

// Zone cliccabili (Piano 15, Task 3, D15-T3): generalizzano linkSpan di
// internal/ui/thread/links.go. Una Zone e la porzione visibile di un elemento
// cliccabile (link, bottone, chip allegato, immagine) in una riga resa; Out
// porta le righe gia stilizzate e, riga per riga, le zone. E l'unica mappa clic:
// LinkAt/LinkClick del thread la interrogano (Task 14), nessuna seconda mappa.

// ZoneKind e il tipo di elemento cliccabile di una Zone.
type ZoneKind uint8

const (
	// ZoneLink: link nel testo.
	ZoneLink ZoneKind = iota
	// ZoneButton: riquadro del bottone (D15-01).
	ZoneButton
	// ZoneAttachment: chip allegato (D15-06); Index e l'indice dell'allegato.
	ZoneAttachment
	// ZoneImage: chip immagine (D15-08); di norma senza zona, riservato.
	ZoneImage
)

// Zone e una porzione cliccabile di una riga: colonne [X0,X1), come linkSpan.
// Le colonne sono celle del terminale (lipgloss.Width), mai byte o rune: le
// sequenze OSC 8 e gli stili non contano.
type Zone struct {
	X0, X1 int
	Kind   ZoneKind
	URL    string // destinazione gia sanificata (D15-T13); vuota per ZoneAttachment
	Index  int    // solo ZoneAttachment: indice dell'allegato
	ID     string // id OSC 8 comune a tutte le porzioni dello stesso elemento
	Warn   bool   // testo con dominio diverso dal reale (D15-07, D15-T6)
}

// Out e il risultato di un renderer: righe gia stilizzate e, per ciascuna, le
// zone cliccabili. Invariante: len(Zones) == len(Lines); una riga senza zone ha
// Zones[i] == nil. Out{} e il risultato vuoto valido.
type Out struct {
	Lines []string
	Zones [][]Zone
}

// norm ritorna le zone di o riallineate alle righe: len(Zones) pareggiata a
// len(Lines) (zone mancanti = nil, zone in eccesso scartate) e slice interne
// copiate. Cosi Stack e Indent producono sempre un Out valido e indipendente
// dagli argomenti, anche partendo da un Out costruito a mano.
func (o Out) norm() [][]Zone {
	z := make([][]Zone, len(o.Lines))
	for i := range z {
		if i < len(o.Zones) && o.Zones[i] != nil {
			z[i] = append([]Zone(nil), o.Zones[i]...)
		}
	}
	return z
}

// Stack concatena i risultati uno sotto l'altro, nell'ordine dato (Piano 15,
// Task 3). Le zone non cambiano: le colonne sono relative alla riga. Non
// modifica gli argomenti e non ne condivide la memoria.
func Stack(outs ...Out) Out {
	var r Out
	for _, o := range outs {
		r.Lines = append(r.Lines, o.Lines...)
		r.Zones = append(r.Zones, o.norm()...)
	}
	return r
}

// Indent prefissa ogni riga (anche quelle vuote) con pad spazi e sposta di pad
// colonne le zone, cosi X0 e X1 restano coerenti con la riga resa (Piano 15,
// Task 3). Con pad <= 0 le righe restano invariate. Non modifica l'argomento.
func Indent(o Out, pad int) Out {
	if pad < 0 {
		pad = 0
	}
	prefix := strings.Repeat(" ", pad)
	r := Out{Lines: make([]string, len(o.Lines)), Zones: o.norm()}
	for i, l := range o.Lines {
		r.Lines[i] = prefix + l
	}
	for _, zs := range r.Zones {
		for k := range zs {
			zs[k].X0 += pad
			zs[k].X1 += pad
		}
	}
	return r
}

// ZoneAt ritorna la zona che copre la colonna x di una riga, se c'e: intervallo
// semiaperto [X0,X1), prima zona in ordine (Piano 15, Task 3). E la parte
// geometrica di LinkAt del thread: il chiamante porta x nelle coordinate della
// riga, scorrimento orizzontale compreso.
func ZoneAt(zs []Zone, x int) (Zone, bool) {
	for _, z := range zs {
		if x >= z.X0 && x < z.X1 {
			return z, true
		}
	}
	return Zone{}, false
}
