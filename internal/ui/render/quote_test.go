package render

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/andsigno82/gmail-tui/internal/msg/richtext"
)

func para(runs ...richtext.Run) richtext.Block {
	return richtext.Block{Kind: richtext.KindParagraph, Runs: runs}
}

func quote(depth int, children ...richtext.Block) richtext.Block {
	return richtext.Block{Kind: richtext.KindQuote, Depth: depth, Children: children}
}

func barStyles() []lipgloss.Style {
	return []lipgloss.Style{
		lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("4")),
	}
}

// TestQuoteSemplice (Piano 15, Task 8): ogni riga ha la barra, il testo va a
// capo sulla larghezza meno la barra.
func TestQuoteSemplice(t *testing.T) {
	b := quote(0, para(richtext.Run{Text: "uno due tre quattro cinque sei sette otto"}))
	o := Quote(b, testCtx(t, 16))
	valid(t, o)
	if len(o.Lines) < 3 {
		t.Fatalf("attese piu righe a capo: %q", o.Lines)
	}
	for i, l := range o.Lines {
		if !strings.HasPrefix(plain(l), "▎ ") {
			t.Errorf("riga %d senza barra: %q", i, plain(l))
		}
		if lipgloss.Width(l) > 16 {
			t.Errorf("riga %d larga %d > 16", i, lipgloss.Width(l))
		}
	}
	got := strings.Fields(strings.ReplaceAll(joinPlain(o), "▎", ""))
	if strings.Join(got, " ") != "uno due tre quattro cinque sei sette otto" {
		t.Errorf("testo alterato: %q", got)
	}
}

// TestQuoteAnnidataHTML: blockquote dentro blockquote (Depth 0 poi 1): le righe
// interne hanno due barre, quelle esterne una.
func TestQuoteAnnidataHTML(t *testing.T) {
	b := quote(0,
		para(richtext.Run{Text: "esterna"}),
		quote(1, para(richtext.Run{Text: "interna"})),
	)
	o := Quote(b, testCtx(t, 30))
	valid(t, o)
	var est, dentro bool
	for _, l := range o.Lines {
		p := plain(l)
		switch {
		case strings.Contains(p, "esterna"):
			est = strings.HasPrefix(p, "▎ esterna")
		case strings.Contains(p, "interna"):
			dentro = strings.HasPrefix(p, "▎ ▎ interna")
		}
	}
	if !est || !dentro {
		t.Errorf("barre sbagliate:\n%s", joinPlain(o))
	}
}

// TestQuoteAnnidataUnBasedHTML: con Depth 1-based (1 poi 2) il risultato e lo
// stesso: il numero di barre non dipende dalla convenzione del parser.
func TestQuoteAnnidataUnBasedHTML(t *testing.T) {
	b := quote(1, para(richtext.Run{Text: "esterna"}), quote(2, para(richtext.Run{Text: "interna"})))
	s := joinPlain(Quote(b, testCtx(t, 30)))
	if !strings.Contains(s, "▎ esterna") || !strings.Contains(s, "▎ ▎ interna") || strings.Contains(s, "▎ ▎ ▎") {
		t.Errorf("barre sbagliate:\n%s", s)
	}
}

// TestQuoteTreLivelli: una citazione piatta di Depth 3 (">>>" da FromText) ha
// tre barre; la barra i usa QuoteBar[i % len], a cicli.
func TestQuoteTreLivelli(t *testing.T) {
	c := testCtx(t, 30)
	c.St.QuoteBar = barStyles()
	o := Quote(quote(3, para(richtext.Run{Text: "testo"})), c)
	valid(t, o)
	if len(o.Lines) != 1 {
		t.Fatalf("attesa 1 riga: %q", o.Lines)
	}
	if !strings.HasPrefix(plain(o.Lines[0]), "▎ ▎ ▎ testo") {
		t.Errorf("attese tre barre: %q", plain(o.Lines[0]))
	}
	pos := -1
	for i := range 3 {
		k := strings.Index(o.Lines[0], c.St.QuoteBar[i].Render("▎"))
		if k <= pos {
			t.Errorf("barra %d non nello stile QuoteBar[%d] o fuori ordine in %q", i, i, o.Lines[0])
		}
		pos = k
	}

	// Ciclo: con 4 stili, il livello 4 torna a QuoteBar[0].
	o = Quote(quote(5, para(richtext.Run{Text: "x"})), c)
	valid(t, o)
	if len(o.Lines) == 0 {
		t.Fatal("nessuna riga")
	}
	if n := strings.Count(plain(o.Lines[0]), "▎"); n != 5 {
		t.Fatalf("attese 5 barre, trovate %d", n)
	}
	if n := strings.Count(o.Lines[0], c.St.QuoteBar[0].Render("▎")); n != 2 {
		t.Errorf("la barra 0 compare al livello 0 e 4: %d volte in %q", n, o.Lines[0])
	}
}

// TestQuoteAnnidataColori: nel blockquote annidato la barra interna usa il
// colore del suo livello assoluto (1), non quello del padre.
func TestQuoteAnnidataColori(t *testing.T) {
	c := testCtx(t, 30)
	c.St.QuoteBar = barStyles()
	o := Quote(quote(0, quote(1, para(richtext.Run{Text: "dentro"}))), c)
	valid(t, o)
	if len(o.Lines) == 0 {
		t.Fatal("nessuna riga")
	}
	l := o.Lines[0]
	i0 := strings.Index(l, c.St.QuoteBar[0].Render("▎"))
	i1 := strings.Index(l, c.St.QuoteBar[1].Render("▎"))
	if i0 < 0 || i1 <= i0 {
		t.Errorf("attese barre di livello 0 poi 1: %q", l)
	}
}

// TestQuoteZoneSpostate: un link in una citazione di livello 3 ha la zona
// spostata dalle tre barre (6 colonne), anche sulle righe a capo.
func TestQuoteZoneSpostate(t *testing.T) {
	b := quote(3, para(
		richtext.Run{Text: "leggi "},
		lnk("il documento allegato", "https://x.example/q"),
		richtext.Run{Text: " subito"},
	))
	o := Quote(b, testCtx(t, 20))
	valid(t, o)
	if len(o.Lines) < 3 {
		t.Fatalf("attesa piu righe: %q", o.Lines)
	}
	if got := zoneJoined(t, o, "https://x.example/q"); got != "il documento allegato" {
		t.Errorf("le zone non ricompongono il link: %q\n%s", got, joinPlain(o))
	}
	for i, zs := range o.Zones {
		for _, z := range zs {
			if z.X0 < 6 {
				t.Errorf("riga %d: zona %+v dentro le barre", i, z)
			}
		}
	}
}

// TestQuoteConElencoDentro: un elenco dentro una citazione prende la barra.
func TestQuoteConElencoDentro(t *testing.T) {
	l := richtext.Block{Kind: richtext.KindList, Children: []richtext.Block{para(richtext.Run{Text: "uno"}), para(richtext.Run{Text: "due"})}}
	o := Quote(quote(0, l), testCtx(t, 20))
	valid(t, o)
	s := joinPlain(o)
	if !strings.Contains(s, "▎ - uno") || !strings.Contains(s, "▎ - due") {
		t.Errorf("elenco senza barra:\n%s", s)
	}
}

// TestQuoteSenzaFigli: citazione vuota = Out vuoto valido; Dispatch instrada.
func TestQuoteSenzaFigli(t *testing.T) {
	o := Quote(quote(0), testCtx(t, 20))
	valid(t, o)
	if len(o.Lines) != 0 {
		t.Errorf("attesa nessuna riga: %q", o.Lines)
	}
	d := Dispatch(quote(0, para(richtext.Run{Text: "x"})), testCtx(t, 20))
	if len(d.Lines) == 0 {
		t.Error("Dispatch non rende la citazione")
	}
}

// TestQuoteBlocchiSeparati: due paragrafi sono separati da una riga con la barra.
func TestQuoteBlocchiSeparati(t *testing.T) {
	o := Quote(quote(0, para(richtext.Run{Text: "a"}), para(richtext.Run{Text: "b"})), testCtx(t, 20))
	valid(t, o)
	if len(o.Lines) != 3 || strings.TrimRight(plain(o.Lines[1]), " ") != "▎" {
		t.Errorf("attesa riga di separazione con barra: %q", o.Lines)
	}
}
