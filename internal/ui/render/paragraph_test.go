package render

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/andsigno82/gmail-tui/internal/msg/richtext"
)

// Piano 15, Task 14 (lavoro A): Paragraph e il motore unico dei Runs. Questi test
// descrivono il comportamento visibile: testo a capo, link con controllo di
// dominio, zone corrette, nessuna iniezione.

func TestParagraphACapoSullaLarghezza(t *testing.T) {
	b := para(richtext.Run{Text: "uno due tre quattro cinque sei sette otto"})
	o := Paragraph(b, testCtx(t, 14))
	valid(t, o)
	if len(o.Lines) < 3 {
		t.Fatalf("attese piu righe: %q", o.Lines)
	}
	for i, l := range o.Lines {
		if lipgloss.Width(l) > 14 {
			t.Errorf("riga %d larga %d > 14", i, lipgloss.Width(l))
		}
	}
	if got := strings.Join(strings.Fields(joinPlain(o)), " "); got != "uno due tre quattro cinque sei sette otto" {
		t.Errorf("testo alterato: %q", got)
	}
}

func TestParagraphLinkZonaEStileTratteggiato(t *testing.T) {
	b := para(richtext.Run{Text: "vai su "}, richtext.Run{Text: "amazon.it", Href: "https://www.amazon.it/x"}, richtext.Run{Text: " ora"})
	o := Paragraph(b, testCtx(t, 40))
	valid(t, o)
	if len(o.Lines) != 1 || len(o.Zones[0]) != 1 {
		t.Fatalf("attesa una riga con una zona: %q %+v", o.Lines, o.Zones)
	}
	z := o.Zones[0][0]
	if z.Kind != ZoneLink || z.URL != "https://www.amazon.it/x" || z.Warn || z.ID == "" {
		t.Fatalf("zona = %+v", z)
	}
	if z.X0 != len("vai su ") || z.X1 != len("vai su amazon.it") {
		t.Errorf("colonne [%d,%d)", z.X0, z.X1)
	}
	if !strings.Contains(o.Lines[0], sgrDashed) {
		t.Errorf("link senza sottolineatura tratteggiata: %q", o.Lines[0])
	}
	if !strings.Contains(o.Lines[0], "\x1b]8;id="+z.ID+";https://www.amazon.it/x\x1b\\") {
		t.Errorf("OSC 8 mancante o con id diverso: %q", o.Lines[0])
	}
}

func TestParagraphLinkPhishingWarn(t *testing.T) {
	b := para(richtext.Run{Text: "paypal.com", Href: "https://evil.example/login"})
	o := Paragraph(b, testCtx(t, 40))
	valid(t, o)
	if !strings.Contains(plain(o.Lines[0]), "⚠") {
		t.Errorf("marcatore warn assente: %q", plain(o.Lines[0]))
	}
	if !strings.Contains(o.Lines[0], sgrCurly) {
		t.Errorf("sottolineatura ondulata assente: %q", o.Lines[0])
	}
	if len(o.Zones[0]) != 1 || !o.Zones[0][0].Warn {
		t.Fatalf("zona senza Warn: %+v", o.Zones[0])
	}
}

func TestParagraphHrefNonSicuroSenzaZona(t *testing.T) {
	for _, href := range []string{"javascript:alert(1)", "file:///etc/passwd", "https:///senza-host"} {
		o := Paragraph(para(richtext.Run{Text: "clicca", Href: href}), testCtx(t, 40))
		valid(t, o)
		if plain(o.Lines[0]) != "clicca" {
			t.Errorf("%s: testo = %q", href, plain(o.Lines[0]))
		}
		if len(o.Zones[0]) != 0 || strings.Contains(o.Lines[0], "\x1b]8;") {
			t.Errorf("%s: link non apribile reso cliccabile: %+v %q", href, o.Zones[0], o.Lines[0])
		}
	}
}

func TestParagraphHrefConEscNonIniettaSequenze(t *testing.T) {
	o := Paragraph(para(richtext.Run{Text: "x", Href: "https://a.example/\x1b]8;;\x07evil"}), testCtx(t, 40))
	valid(t, o)
	if len(o.Zones[0]) != 1 || strings.ContainsAny(o.Zones[0][0].URL, "\x1b\x07") {
		t.Fatalf("zona = %+v", o.Zones[0])
	}
	if strings.Count(o.Lines[0], "\x1b]8;") != 2 {
		t.Errorf("sequenze OSC 8 iniettate: %q", o.Lines[0])
	}
}

func TestParagraphTestoSanificatoDaBidiEZeroWidth(t *testing.T) {
	o := Paragraph(para(richtext.Run{Text: "ab" + string(rune(0x202e)) + "c" + string(rune(0x200b)) + "d\x1b[31me"}), testCtx(t, 40))
	valid(t, o)
	if got := plain(o.Lines[0]); got != "abcd[31me" {
		t.Errorf("testo = %q", got)
	}
}

func TestParagraphLinkSenzaTestoMostraURL(t *testing.T) {
	o := Paragraph(para(richtext.Run{Href: "https://a.example/p"}), testCtx(t, 40))
	valid(t, o)
	if len(o.Lines) != 1 || !strings.Contains(plain(o.Lines[0]), "https://a.example/p") {
		t.Fatalf("righe = %q", o.Lines)
	}
}

func TestParagraphPreConservaGliACapo(t *testing.T) {
	b := richtext.Block{Kind: richtext.KindPre, Runs: []richtext.Run{{Text: "a  b\n  c"}}}
	o := Paragraph(b, testCtx(t, 40))
	valid(t, o)
	if len(o.Lines) != 2 || plain(o.Lines[0]) != "a  b" || plain(o.Lines[1]) != "  c" {
		t.Fatalf("righe = %q", o.Lines)
	}
}

// Un elemento di elenco HTML e un paragrafo con Children: i blocchi annidati
// stanno sotto il suo testo (richtext/html.go, item).
func TestParagraphConFigliRendeIBlocchiAnnidati(t *testing.T) {
	b := para(richtext.Run{Text: "padre"})
	b.Children = []richtext.Block{quote(0, para(richtext.Run{Text: "citato"}))}
	o := Paragraph(b, testCtx(t, 40))
	valid(t, o)
	if len(o.Lines) != 2 || plain(o.Lines[0]) != "padre" || !strings.Contains(plain(o.Lines[1]), "▎ citato") {
		t.Fatalf("righe = %q", o.Lines)
	}
}

func TestListElementoConFigliAnnidatiHTML(t *testing.T) {
	item := para(richtext.Run{Text: "uno"})
	item.Children = []richtext.Block{{Kind: richtext.KindList, Children: []richtext.Block{para(richtext.Run{Text: "interno"})}}}
	o := List(richtext.Block{Kind: richtext.KindList, Children: []richtext.Block{item}}, testCtx(t, 40))
	valid(t, o)
	if got := joinPlain(o); !strings.Contains(got, "interno") {
		t.Fatalf("elenco annidato perso: %q", got)
	}
}

// Un solo motore: citazioni, elenchi e tabelle applicano lo stesso controllo di
// dominio e la stessa sanificazione di Paragraph.
func TestMotoreUnicoPhishingInQuoteListTable(t *testing.T) {
	runs := []richtext.Run{{Text: "paypal.com", Href: "https://evil.example/"}}
	c := testCtx(t, 40)
	cases := map[string]Out{
		"quote": Quote(quote(0, para(runs...)), c),
		"list":  List(richtext.Block{Kind: richtext.KindList, Children: []richtext.Block{para(runs...)}}, c),
		"table": Table(richtext.Block{Kind: richtext.KindTable, Rows: [][]richtext.Cell{{{Runs: runs}}}}, c),
	}
	for name, o := range cases {
		valid(t, o)
		warn := false
		for _, zs := range o.Zones {
			for _, z := range zs {
				warn = warn || z.Warn
			}
		}
		if !warn || !strings.Contains(joinPlain(o), "⚠") {
			t.Errorf("%s: link sospetto senza Warn/marcatore: %q %+v", name, joinPlain(o), o.Zones)
		}
	}
}
