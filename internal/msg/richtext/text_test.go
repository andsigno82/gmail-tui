package richtext

import (
	"fmt"
	"strings"
	"testing"
)

// rlo e' U+202E (right-to-left override), costruito da codice per non avere
// caratteri invisibili nel sorgente.
var rlo = string(rune(0x202E))

// para ritorna il testo di un paragrafo atteso.
func para(t *testing.T, b Block) string {
	t.Helper()
	if b.Kind != KindParagraph {
		t.Fatalf("kind %d, atteso paragrafo: %+v", b.Kind, b)
	}
	return runsText(b.Runs)
}

func TestFromTextParagrafi(t *testing.T) {
	blocks := FromText("Ciao Mario,\nspero stia bene.\n\n  riga indentata\n\n\nFine")
	if len(blocks) != 3 {
		t.Fatalf("blocchi = %+v", blocks)
	}
	if got := para(t, blocks[0]); got != "Ciao Mario,\nspero stia bene." {
		t.Errorf("primo = %q (le righe a capo si conservano)", got)
	}
	if got := para(t, blocks[1]); got != "  riga indentata" {
		t.Errorf("secondo = %q (l'indentazione si conserva)", got)
	}
	if got := para(t, blocks[2]); got != "Fine" {
		t.Errorf("terzo = %q", got)
	}
}

func TestFromTextVuoto(t *testing.T) {
	for _, in := range []string{"", "\n\n", "   \n "} {
		if got := FromText(in); len(got) != 0 {
			t.Errorf("FromText(%q) = %+v, atteso nessun blocco", in, got)
		}
	}
}

func TestFromTextCitazione(t *testing.T) {
	blocks := FromText("Risposta\n> primo\n> secondo\n>\n> terzo\nDopo")
	if len(blocks) != 3 || blocks[1].Kind != KindQuote {
		t.Fatalf("blocchi = %+v", blocks)
	}
	q := blocks[1]
	if q.Depth != 0 || len(q.Children) != 2 {
		t.Fatalf("citazione = %+v", q)
	}
	if para(t, q.Children[0]) != "primo\nsecondo" || para(t, q.Children[1]) != "terzo" {
		t.Errorf("figli = %+v", q.Children)
	}
	if para(t, blocks[0]) != "Risposta" || para(t, blocks[2]) != "Dopo" {
		t.Errorf("testo attorno = %+v", blocks)
	}
}

// TestFromTextCitazioneAnnidata: ">>" produce due livelli annidati come
// blockquote in HTML (Depth 0 esterno, 1 interno): un solo modello per i
// renderer. Il piano scrive "Depth 2" per ">>": qui Depth e' 0-based come in
// model.go, quindi il livello interno ha Depth 1 e la profondita' totale e' 2.
func TestFromTextCitazioneAnnidata(t *testing.T) {
	for _, in := range []string{">> due", "> > due", " >  > due"} {
		blocks := FromText(in)
		if len(blocks) != 1 || blocks[0].Kind != KindQuote || blocks[0].Depth != 0 {
			t.Fatalf("%q: blocchi = %+v", in, blocks)
		}
		in1 := blocks[0].Children
		if len(in1) != 1 || in1[0].Kind != KindQuote || in1[0].Depth != 1 {
			t.Fatalf("%q: interno = %+v", in, in1)
		}
		if para(t, in1[0].Children[0]) != "due" {
			t.Errorf("%q: testo = %+v", in, in1[0].Children)
		}
	}
}

func TestFromTextCitazioniMisteDepth(t *testing.T) {
	blocks := FromText("> a\n>> b\n> c\nd")
	// Quote0[ a, Quote1[b], c ], d
	if len(blocks) != 2 || blocks[0].Kind != KindQuote {
		t.Fatalf("blocchi = %+v", blocks)
	}
	kids := blocks[0].Children
	if len(kids) != 3 || kids[1].Kind != KindQuote || kids[1].Depth != 1 {
		t.Fatalf("figli = %+v", kids)
	}
	if para(t, kids[0]) != "a" || para(t, kids[2]) != "c" || para(t, blocks[1]) != "d" {
		t.Errorf("testo = %+v", blocks)
	}
}

func TestFromTextCitazioneLimiteProfondita(t *testing.T) {
	blocks := FromText(strings.Repeat(">", 40) + " fondo")
	if d := depthOf(blocks); d > MaxDepth+1 {
		t.Errorf("annidamento = %d, oltre MaxDepth", d)
	}
	if !strings.Contains(allText(blocks), "fondo") {
		t.Error("testo perso")
	}
}

func TestFromTextElenchi(t *testing.T) {
	blocks := FromText("Da fare:\n- uno\n* due\n- tre\n\ndopo")
	if len(blocks) != 3 || blocks[1].Kind != KindList {
		t.Fatalf("blocchi = %+v", blocks)
	}
	l := blocks[1]
	if l.Ordered || len(l.Children) != 3 || para(t, l.Children[1]) != "due" {
		t.Errorf("elenco = %+v", l)
	}
}

func TestFromTextElencoOrdinato(t *testing.T) {
	blocks := FromText("1. primo\n2. secondo\n3) terzo")
	if len(blocks) != 1 || blocks[0].Kind != KindList {
		t.Fatalf("blocchi = %+v", blocks)
	}
	l := blocks[0]
	if !l.Ordered || l.Start != 1 || len(l.Children) != 3 {
		t.Errorf("elenco = %+v", l)
	}
	blocks = FromText("3. solo\n4. dopo")
	if blocks[0].Start != 3 {
		t.Errorf("Start = %d, atteso 3", blocks[0].Start)
	}
}

func TestFromTextElenchiDiversiSiDividono(t *testing.T) {
	blocks := FromText("- a\n1. b\n- c")
	if len(blocks) != 3 {
		t.Fatalf("blocchi = %+v", blocks)
	}
	if blocks[0].Ordered || !blocks[1].Ordered || blocks[2].Ordered {
		t.Errorf("ordinamento = %v %v %v", blocks[0].Ordered, blocks[1].Ordered, blocks[2].Ordered)
	}
}

func TestFromTextNonSonoElenchi(t *testing.T) {
	for _, in := range []string{"-senza spazio", "*enfasi*", "1.5 litri", "-", "12345. troppo lungo"} {
		blocks := FromText(in)
		if len(blocks) != 1 || blocks[0].Kind != KindParagraph {
			t.Errorf("%q: blocchi = %+v, atteso un paragrafo", in, blocks)
		}
	}
}

func TestFromTextElencoContinuazione(t *testing.T) {
	blocks := FromText("- primo elemento\n  che continua\n- secondo")
	if len(blocks) != 1 || len(blocks[0].Children) != 2 {
		t.Fatalf("blocchi = %+v", blocks)
	}
	if got := para(t, blocks[0].Children[0]); got != "primo elemento che continua" {
		t.Errorf("elemento = %q", got)
	}
}

func TestFromTextElencoInCitazione(t *testing.T) {
	blocks := FromText("> - a\n> - b")
	if len(blocks) != 1 || blocks[0].Kind != KindQuote {
		t.Fatalf("blocchi = %+v", blocks)
	}
	if l := blocks[0].Children; len(l) != 1 || l[0].Kind != KindList || len(l[0].Children) != 2 {
		t.Errorf("figli = %+v", l)
	}
}

func TestFromTextLink(t *testing.T) {
	tests := []struct {
		name, in string
		want     []Run
	}{
		{"punteggiatura finale fuori", "vai su https://example.com/a, poi", []Run{{Text: "vai su "}, {Text: "https://example.com/a", Href: "https://example.com/a"}, {Text: ", poi"}}},
		{"punto finale", "leggi https://example.com/x.", []Run{{Text: "leggi "}, {Text: "https://example.com/x", Href: "https://example.com/x"}, {Text: "."}}},
		{"parentesi", "(https://example.com/a)", []Run{{Text: "("}, {Text: "https://example.com/a", Href: "https://example.com/a"}, {Text: ")"}}},
		{"parentesi interna", "https://example.com/a_(b).", []Run{{Text: "https://example.com/a_(b)", Href: "https://example.com/a_(b)"}, {Text: "."}}},
		{"mailto", "scrivi a mailto:info@example.org!", []Run{{Text: "scrivi a "}, {Text: "mailto:info@example.org", Href: "mailto:info@example.org"}, {Text: "!"}}},
		{"angolari", "<https://example.com/a>", []Run{{Text: "<"}, {Text: "https://example.com/a", Href: "https://example.com/a"}, {Text: ">"}}},
		{"due link", "http://example.com e https://example.org", []Run{{Text: "http://example.com", Href: "http://example.com"}, {Text: " e "}, {Text: "https://example.org", Href: "https://example.org"}}},
		{"schema maiuscolo", "HTTPS://Example.com/", []Run{{Text: "HTTPS://Example.com/", Href: "HTTPS://Example.com/"}}},
		{"nessun link", "javascript:alert(1) e ftp://example.com", []Run{{Text: "javascript:alert(1) e ftp://example.com"}}},
		{"RLO nell'URL rimosso", "vedi https://exa" + rlo + "mple.com/x ora", []Run{{Text: "vedi "}, {Text: "https://example.com/x", Href: "https://example.com/x"}, {Text: " ora"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks := FromText(tt.in)
			if len(blocks) != 1 {
				t.Fatalf("blocchi = %+v", blocks)
			}
			if fmt.Sprint(blocks[0].Runs) != fmt.Sprint(tt.want) {
				t.Errorf("runs = %q\natteso %q", blocks[0].Runs, tt.want)
			}
		})
	}
}

func TestFromTextSanifica(t *testing.T) {
	blocks := FromText("a\x1b[31mb\x07\r\nc\td" + rlo + "e")
	if len(blocks) != 1 {
		t.Fatalf("blocchi = %+v", blocks)
	}
	if got := runsText(blocks[0].Runs); got != "a[31mb\nc de" {
		t.Errorf("testo = %q", got)
	}
}

func TestFromTextTroncamento(t *testing.T) {
	in := strings.Repeat("a", MaxInputBytes+10) + "\n\nFINE"
	blocks := FromText(in)
	if strings.Contains(allText(blocks), "FINE") {
		t.Error("testo oltre il limite presente")
	}
	last := blocks[len(blocks)-1]
	if !strings.Contains(runsText(last.Runs), "messaggio troncato") {
		t.Errorf("manca la dichiarazione: %+v", last)
	}
	if b := FromText(strings.Repeat("a", MaxInputBytes)); strings.Contains(allText(b), "troncato") {
		t.Error("troncamento dichiarato su input pari al limite")
	}
}
