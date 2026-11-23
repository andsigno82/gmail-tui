package render

import (
	"strings"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/msg/richtext"
)

func items(texts ...string) []richtext.Block {
	r := make([]richtext.Block, len(texts))
	for i, s := range texts {
		r[i] = para(richtext.Run{Text: s})
	}
	return r
}

func ulist(ch ...richtext.Block) richtext.Block {
	return richtext.Block{Kind: richtext.KindList, Children: ch}
}

func olist(start int, ch ...richtext.Block) richtext.Block {
	return richtext.Block{Kind: richtext.KindList, Ordered: true, Start: start, Children: ch}
}

// trimmed ritorna le righe senza escape e senza spazi finali.
func trimmed(o Out) []string {
	r := make([]string, len(o.Lines))
	for i, l := range o.Lines {
		r[i] = strings.TrimRight(plain(l), " ")
	}
	return r
}

func eqLines(t *testing.T, o Out, want ...string) {
	t.Helper()
	valid(t, o)
	got := trimmed(o)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("righe:\n%q\nattese:\n%q", got, want)
	}
}

// TestListPuntato (Piano 15, Task 8): enumeratore list.Dash.
func TestListPuntato(t *testing.T) {
	eqLines(t, List(ulist(items("uno", "due")...), testCtx(t, 20)), "- uno", "- due")
}

// TestListOrdinataDaStart: Start 3 -> "3.", "4."; Start 9 allinea le cifre.
func TestListOrdinataDaStart(t *testing.T) {
	eqLines(t, List(olist(3, items("a", "b")...), testCtx(t, 20)), "3. a", "4. b")
	eqLines(t, List(olist(9, items("a", "b")...), testCtx(t, 20)), " 9. a", "10. b")
}

// TestListOrdinataStartZero: il valore zero del modello (Start non impostato)
// parte da 1, come <ol> senza attributo.
func TestListOrdinataStartZero(t *testing.T) {
	eqLines(t, List(olist(0, items("a", "b")...), testCtx(t, 20)), "1. a", "2. b")
}

// TestListACapo: la riga lunga va a capo sotto il testo, non sotto il marcatore,
// e non supera la larghezza.
func TestListACapo(t *testing.T) {
	o := List(ulist(items("uno due tre quattro cinque")...), testCtx(t, 12))
	valid(t, o)
	if len(o.Lines) < 3 {
		t.Fatalf("attese piu righe: %q", trimmed(o))
	}
	for i, l := range trimmed(o) {
		want := "  "
		if i == 0 {
			want = "- "
		}
		if !strings.HasPrefix(l, want) || strings.HasPrefix(l, "   ") {
			t.Errorf("riga %d: %q", i, l)
		}
	}
	if maxWidth(o) > 12 {
		t.Errorf("larghezza %d > 12", maxWidth(o))
	}
}

// TestListAnnidata: un elenco ordinato dentro uno puntato sta sotto il testo
// dell'elemento precedente, con numerazione propria; il genitore ordinato non
// conta l'elenco annidato come elemento.
func TestListAnnidata(t *testing.T) {
	o := List(ulist(
		para(richtext.Run{Text: "frutta"}),
		olist(5, items("mela", "pera")...),
		para(richtext.Run{Text: "verdura"}),
	), testCtx(t, 30))
	eqLines(t, o, "- frutta", "  5. mela", "  6. pera", "- verdura")

	o = List(olist(1,
		para(richtext.Run{Text: "primo"}),
		ulist(items("x", "y")...),
		para(richtext.Run{Text: "secondo"}),
	), testCtx(t, 30))
	eqLines(t, o, "1. primo", "   - x", "   - y", "2. secondo")
}

// TestListAnnidataTreLivelli: l'annidamento e ricorsivo.
func TestListAnnidataTreLivelli(t *testing.T) {
	o := List(ulist(
		para(richtext.Run{Text: "a"}),
		ulist(
			para(richtext.Run{Text: "b"}),
			olist(1, items("c")...),
		),
	), testCtx(t, 30))
	eqLines(t, o, "- a", "  - b", "    1. c")
}

// TestListZone: i link negli elementi hanno la zona giusta dopo il marcatore,
// sulle righe a capo e negli elenchi annidati.
func TestListZone(t *testing.T) {
	b := ulist(
		para(richtext.Run{Text: "apri "}, lnk("la pagina del progetto", "https://x.example/p")),
		olist(7, para(lnk("nidificato", "https://x.example/n"))),
	)
	o := List(b, testCtx(t, 14))
	valid(t, o)
	if got := zoneJoined(t, o, "https://x.example/p"); got != "la pagina del progetto" {
		t.Errorf("zone del primo link: %q\n%s", got, joinPlain(o))
	}
	// "nidificato" (10 colonne) non entra nei 9 disponibili: si spezza.
	if got := strings.ReplaceAll(zoneJoined(t, o, "https://x.example/n"), " ", ""); got != "nidificato" {
		t.Errorf("zona annidata: %q\n%s", got, joinPlain(o))
	}
	for i, zs := range o.Zones {
		for _, z := range zs {
			if z.X0 < 2 {
				t.Errorf("riga %d: zona %+v dentro il marcatore", i, z)
			}
		}
	}
	if maxWidth(o) > 14 {
		t.Errorf("larghezza %d > 14", maxWidth(o))
	}
}

// TestListVuotaEDispatch: elenco vuoto = Out vuoto valido; Dispatch instrada.
func TestListVuotaEDispatch(t *testing.T) {
	o := List(ulist(), testCtx(t, 20))
	valid(t, o)
	if len(o.Lines) != 0 {
		t.Errorf("attesa nessuna riga: %q", o.Lines)
	}
	eqLines(t, Dispatch(ulist(items("x")...), testCtx(t, 20)), "- x")
}
