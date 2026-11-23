package richtext

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixture legge una fixture anonimizzata da testdata/html (Piano 15, Task 4).
func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "html", name))
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return string(b)
}

// runsText concatena il testo dei run.
func runsText(runs []Run) string {
	var sb strings.Builder
	for _, r := range runs {
		sb.WriteString(r.Text)
	}
	return sb.String()
}

// allText concatena il testo di tutti i blocchi (figli, celle, immagini)
// separando i blocchi con "|": serve alle asserzioni "non compare".
func allText(blocks []Block) string {
	var sb strings.Builder
	for _, b := range blocks {
		sb.WriteString(runsText(b.Runs))
		sb.WriteString(b.Alt)
		for _, row := range b.Rows {
			for _, c := range row {
				sb.WriteString(runsText(c.Runs))
				sb.WriteString(" ")
			}
		}
		sb.WriteString(allText(b.Children))
		sb.WriteString("|")
	}
	return sb.String()
}

// collect raccoglie ricorsivamente i blocchi del Kind richiesto.
func collect(blocks []Block, k Kind) []Block {
	var out []Block
	for _, b := range blocks {
		if b.Kind == k {
			out = append(out, b)
		}
		out = append(out, collect(b.Children, k)...)
	}
	return out
}

// links raccoglie tutti i run con Href (paragrafi, celle, figli), bottoni esclusi.
func links(blocks []Block) []Run {
	var out []Run
	for _, b := range blocks {
		for _, r := range b.Runs {
			if r.Href != "" && b.Kind != KindButton {
				out = append(out, r)
			}
		}
		for _, row := range b.Rows {
			for _, c := range row {
				for _, r := range c.Runs {
					if r.Href != "" {
						out = append(out, r)
					}
				}
			}
		}
		out = append(out, links(b.Children)...)
	}
	return out
}

// depthOf è la profondità massima di annidamento dei blocchi (Children).
func depthOf(blocks []Block) int {
	max := 0
	for _, b := range blocks {
		if d := 1 + depthOf(b.Children); d > max {
			max = d
		}
	}
	return max
}

func TestFromHTMLParagrafi(t *testing.T) {
	blocks, truncated := FromHTML("<p>uno</p><div>due  <b>tre</b>\n quattro</div>testo<br>dopo")
	if truncated {
		t.Fatal("truncated inatteso")
	}
	var got []string
	for _, b := range blocks {
		if b.Kind != KindParagraph {
			t.Fatalf("kind %d, atteso paragrafo", b.Kind)
		}
		got = append(got, runsText(b.Runs))
	}
	want := []string{"uno", "due tre quattro", "testo", "dopo"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("paragrafi = %q, attesi %q", got, want)
	}
}

func TestFromHTMLIgnoraContenutoNonVisibile(t *testing.T) {
	src := `<html><head><title>TITOLO</title><style>.a{}</style></head><body>
<script>alert("SCRIPT")</script><noscript>NOSCRIPT</noscript>
<iframe src="https://x.example.com">IFRAME</iframe><object>OBJECT</object><embed>
<svg><text>SVG</text></svg><form><input value="FORM">FORMTXT</form>
<p hidden>NASCOSTO</p><p>visibile</p></body></html>`
	blocks, _ := FromHTML(src)
	got := allText(blocks)
	for _, s := range []string{"TITOLO", "SCRIPT", "NOSCRIPT", "IFRAME", "OBJECT", "SVG", "FORM", "NASCOSTO", ".a{}"} {
		if strings.Contains(got, s) {
			t.Errorf("%q non deve comparire: %q", s, got)
		}
	}
	if !strings.Contains(got, "visibile") {
		t.Errorf("manca il testo visibile: %q", got)
	}
}

func TestFromHTMLLinkInParagrafo(t *testing.T) {
	blocks, _ := FromHTML(fixture(t, "inline_link.html"))
	var hrefs []string
	for _, r := range links(blocks) {
		hrefs = append(hrefs, r.Href)
	}
	want := []string{
		"https://www.paypal.com.verifica.example.net/login",
		"https://www.example.com/it/",
		"mailto:assistenza@example.org",
	}
	if fmt.Sprint(hrefs) != fmt.Sprint(want) {
		t.Errorf("href = %q, attesi %q (javascript: e relativi esclusi)", hrefs, want)
	}
	if blocks[0].Kind != KindParagraph || runsText(blocks[0].Runs) != "Accedi su paypal.com per verificare." {
		t.Errorf("testo del primo paragrafo = %q", runsText(blocks[0].Runs))
	}
	got := allText(blocks)
	if !strings.Contains(got, "link finto") || !strings.Contains(got, "relativo") {
		t.Errorf("il testo dei link non sicuri deve restare: %q", got)
	}
}

func TestFromHTMLLinkSenzaTestoUsaHref(t *testing.T) {
	blocks, _ := FromHTML(`<p><a href="https://example.com/x"> </a></p>`)
	ls := links(blocks)
	if len(ls) != 1 || strings.TrimSpace(ls[0].Text) != "https://example.com/x" {
		t.Errorf("link = %+v, atteso testo = href", ls)
	}
}

func TestFromHTMLImmagini(t *testing.T) {
	blocks, _ := FromHTML(fixture(t, "img_alt.html"))
	var alts []string
	for _, b := range collect(blocks, KindImage) {
		alts = append(alts, b.Alt)
	}
	// vacanza (alt), senza-alt (alt vuoto); il banner nel link e' testo di
	// link; il pixel 1x1 e' scartato.
	want := []string{"Tramonto sul lago", ""}
	if fmt.Sprint(alts) != fmt.Sprint(want) {
		t.Errorf("alt = %q, attesi %q", alts, want)
	}
	ls := links(blocks)
	if len(ls) != 1 || ls[0].Text != "Banner promo estate" {
		t.Errorf("immagine in link = %+v, atteso testo = alt", ls)
	}
}

// TestFromHTMLNessunSrc (D15-T13): il src delle immagini non finisce mai nel
// modello, ne' come testo ne' come href: nessuna richiesta di rete possibile.
func TestFromHTMLNessunSrc(t *testing.T) {
	for _, name := range []string{"img_alt.html", "newsletter.html"} {
		blocks, _ := FromHTML(fixture(t, name))
		dump := fmt.Sprintf("%+v", blocks)
		for _, s := range []string{"img.example.com", "cdn.example.com", "track.example.org", ".png", ".gif", ".jpg"} {
			if strings.Contains(dump, s) {
				t.Errorf("%s: %q nel modello: %s", name, s, dump)
			}
		}
	}
}

func TestFromHTMLCitazioniAnnidate(t *testing.T) {
	blocks, _ := FromHTML(fixture(t, "quote_nested.html"))
	if len(blocks) != 2 || blocks[0].Kind != KindParagraph || blocks[1].Kind != KindQuote {
		t.Fatalf("struttura = %+v", blocks)
	}
	q0 := blocks[1]
	if q0.Depth != 0 {
		t.Errorf("Depth esterna = %d, atteso 0", q0.Depth)
	}
	// figli: paragrafo, citazione (Depth 1), paragrafo.
	if len(q0.Children) != 3 || q0.Children[1].Kind != KindQuote || q0.Children[1].Depth != 1 {
		t.Fatalf("figli = %+v", q0.Children)
	}
	q2 := q0.Children[1].Children
	if len(q2) != 2 || q2[1].Kind != KindQuote || q2[1].Depth != 2 {
		t.Fatalf("terzo livello = %+v", q2)
	}
	if runsText(q2[1].Children[0].Runs) != "Ho aggiornato il documento." {
		t.Errorf("testo terzo livello = %q", runsText(q2[1].Children[0].Runs))
	}
}

func TestFromHTMLElenchiAnnidati(t *testing.T) {
	blocks, _ := FromHTML(fixture(t, "nested_lists.html"))
	lists := collect(blocks, KindList)
	if len(lists) != 3 { // ul esterna, ul annidata, ol
		t.Fatalf("liste = %d, attese 3", len(lists))
	}
	ul := lists[0]
	if ul.Ordered || len(ul.Children) != 2 {
		t.Fatalf("ul = %+v", ul)
	}
	if runsText(ul.Children[0].Runs) != "Documenti" || len(ul.Children[0].Children) != 1 {
		t.Fatalf("primo elemento = %+v", ul.Children[0])
	}
	sub := ul.Children[0].Children[0]
	if sub.Kind != KindList || len(sub.Children) != 2 || runsText(sub.Children[1].Runs) != "Codice fiscale" {
		t.Errorf("annidata = %+v", sub)
	}
	ol := lists[2]
	if !ol.Ordered || ol.Start != 3 || len(ol.Children) != 2 {
		t.Fatalf("ol = %+v", ol)
	}
	if ls := links(ol.Children[1:]); len(ls) != 1 || ls[0].Href != "https://help.example.org/passo-4" {
		t.Errorf("link nell'elemento = %+v", ls)
	}
}

func TestFromHTMLOlStartDefault(t *testing.T) {
	blocks, _ := FromHTML("<ol><li>a</li></ol><ul><li>b</li></ul>")
	if len(blocks) != 2 {
		t.Fatalf("blocchi = %+v", blocks)
	}
	if blocks[0].Start != 1 || !blocks[0].Ordered {
		t.Errorf("ol = %+v, atteso Start 1", blocks[0])
	}
	if blocks[1].Start != 0 || blocks[1].Ordered {
		t.Errorf("ul = %+v", blocks[1])
	}
}

func TestFromHTMLTabellaFattura(t *testing.T) {
	blocks, _ := FromHTML(fixture(t, "invoice_table.html"))
	tables := collect(blocks, KindTable)
	if len(tables) != 1 {
		t.Fatalf("tabelle = %d, attesa 1", len(tables))
	}
	rows := tables[0].Rows
	if len(rows) != 4 {
		t.Fatalf("righe = %d, attese 4", len(rows))
	}
	if !rows[0][0].Header || rows[1][0].Header {
		t.Errorf("Header: riga 0 = %v, riga 1 = %v", rows[0][0].Header, rows[1][0].Header)
	}
	if runsText(rows[0][2].Runs) != "Importo" || runsText(rows[1][2].Runs) != "120,00" {
		t.Errorf("celle = %q / %q", runsText(rows[0][2].Runs), runsText(rows[1][2].Runs))
	}
	if runsText(rows[3][1].Runs) != "150,00" {
		t.Errorf("cella annidata in <b> = %q", runsText(rows[3][2].Runs))
	}
}

// TestFromHTMLTabellaDiLayout (D15-T1): le newsletter impaginano con tabelle;
// una tabella con blocchi o bottoni nelle celle, o una cella sola, non e' una
// KindTable: le celle diventano blocchi in sequenza.
func TestFromHTMLTabellaDiLayout(t *testing.T) {
	blocks, _ := FromHTML(fixture(t, "newsletter.html"))
	if n := len(collect(blocks, KindTable)); n != 0 {
		t.Errorf("tabelle di layout = %d, attese 0", n)
	}
	got := allText(blocks)
	for _, s := range []string{"Offerte della settimana", "Ciao Mario", "sfoglia il catalogo", "Annulla l'iscrizione", "Negozio Esempio"} {
		if !strings.Contains(got, s) {
			t.Errorf("manca %q in %q", s, got)
		}
	}
	for _, s := range []string{"Versione Outlook", "window.track"} {
		if strings.Contains(got, s) {
			t.Errorf("%q non deve comparire", s)
		}
	}
}

func TestFromHTMLTabellaDatiConBr(t *testing.T) {
	blocks, _ := FromHTML(`<table><tr><td>a</td><td>b<br>c</td></tr><tr><td>d</td><td>e</td></tr></table>`)
	tb := collect(blocks, KindTable)
	if len(tb) != 1 || runsText(tb[0].Rows[0][1].Runs) != "b c" {
		t.Errorf("tabella = %+v", blocks)
	}
}

// TestFromHTMLTabellaLimiti (D15-T4): oltre 200 righe o 20 colonne la tabella
// si tronca e il troncamento e' dichiarato in un paragrafo dopo la tabella.
// Righe e colonne si provano separatamente: una tabella piena 200x20 supera da
// sola il budget di MaxNodes (vedi TestFromHTMLLimiteNodi).
func TestFromHTMLTabellaLimiti(t *testing.T) {
	grid := func(rows, cols int) string {
		var sb strings.Builder
		sb.WriteString("<table>")
		for r := 0; r < rows; r++ {
			sb.WriteString("<tr>")
			for c := 0; c < cols; c++ {
				sb.WriteString("<td>x</td>")
			}
			sb.WriteString("</tr>")
		}
		sb.WriteString("</table>")
		return sb.String()
	}
	check := func(name string, blocks []Block, wantRows, wantCols int) {
		t.Helper()
		tb := collect(blocks, KindTable)
		if len(tb) != 1 {
			t.Fatalf("%s: tabelle = %d", name, len(tb))
		}
		if len(tb[0].Rows) != wantRows {
			t.Errorf("%s: righe = %d, attese %d", name, len(tb[0].Rows), wantRows)
		}
		for i, row := range tb[0].Rows {
			if len(row) != wantCols {
				t.Fatalf("%s: riga %d: colonne = %d, attese %d", name, i, len(row), wantCols)
			}
		}
		last := blocks[len(blocks)-1]
		if last.Kind != KindParagraph || !strings.Contains(runsText(last.Runs), "tabella troncata") {
			t.Errorf("%s: manca la dichiarazione di troncamento: %+v", name, last)
		}
	}
	blocks, _ := FromHTML(grid(MaxTableRows+50, 2))
	check("righe", blocks, MaxTableRows, 2)
	blocks, _ = FromHTML(grid(5, MaxTableCols+10))
	check("colonne", blocks, 5, MaxTableCols)
}

func TestFromHTMLTabellaSenzaTroncamento(t *testing.T) {
	blocks, _ := FromHTML(fixture(t, "invoice_table.html"))
	if len(blocks) == 0 || strings.Contains(allText(blocks), "troncata") {
		t.Error("dichiarazione di troncamento senza troncamento (o nessun blocco)")
	}
}

func TestFromHTMLPreERegola(t *testing.T) {
	blocks, _ := FromHTML("<pre>riga 1\n  riga 2\n</pre><hr><p>x</p>")
	if len(blocks) != 3 || blocks[0].Kind != KindPre || blocks[1].Kind != KindRule {
		t.Fatalf("blocchi = %+v", blocks)
	}
	if got := runsText(blocks[0].Runs); got != "riga 1\n  riga 2" {
		t.Errorf("pre = %q", got)
	}
}

func TestFromHTMLMalformato(t *testing.T) {
	blocks, truncated := FromHTML(fixture(t, "malformed.html"))
	if truncated {
		t.Error("truncated inatteso")
	}
	got := allText(blocks)
	for _, s := range []string{"Primo paragrafo senza chiusura", "grassetto", "incrociato", "cella senza chiusura", "altra cella"} {
		if !strings.Contains(got, s) {
			t.Errorf("manca %q in %q", s, got)
		}
	}
	for _, in := range []string{"", "<", "<<<>>>", "<p", "</p></div>", "<a href=", "&", "\x00\xff<p>\xc3"} {
		FromHTML(in) // nessun panic
	}
}

func TestFromHTMLTroncamentoInput(t *testing.T) {
	huge := "<p>" + strings.Repeat("a", MaxInputBytes+1000) + "</p><p>FINE</p>"
	blocks, truncated := FromHTML(huge)
	if !truncated {
		t.Fatal("truncated atteso true oltre MaxInputBytes")
	}
	if strings.Contains(allText(blocks), "FINE") {
		t.Error("il testo oltre il limite non deve comparire")
	}
	if got := len(runsText(blocks[0].Runs)); got > MaxInputBytes {
		t.Errorf("testo = %d byte, oltre il limite", got)
	}
	// il taglio non spezza una sequenza UTF-8
	multi := strings.Repeat("è", MaxInputBytes) // 2 byte per rune
	blocks, truncated = FromHTML("<p>" + multi)
	if !truncated {
		t.Fatal("truncated atteso")
	}
	for _, r := range runsText(blocks[0].Runs) {
		if r == '�' {
			t.Fatal("sequenza UTF-8 spezzata dal troncamento")
		}
	}
	// input esattamente al limite: non troncato
	if _, truncated := FromHTML(strings.Repeat("a", MaxInputBytes)); truncated {
		t.Error("truncated su input pari al limite")
	}
}

func TestFromHTMLLimiteNodi(t *testing.T) {
	src := "<p>PRIMO</p>" + strings.Repeat("<span>x</span>", MaxNodes*2) + "<p>ULTIMO</p>"
	blocks, truncated := FromHTML(src)
	if truncated {
		t.Error("truncated riguarda solo i byte")
	}
	if len(blocks) == 0 {
		t.Fatal("nessun blocco")
	}
	got := allText(blocks)
	if !strings.Contains(got, "PRIMO") || strings.Contains(got, "ULTIMO") {
		t.Errorf("nodi oltre il limite non ignorati: %.80q", got)
	}
	last := blocks[len(blocks)-1]
	if !strings.Contains(runsText(last.Runs), "troppi elementi") {
		t.Errorf("manca la dichiarazione: %+v", last)
	}
	if n := strings.Count(got, "x"); n > MaxNodes {
		t.Errorf("x = %d, oltre i nodi ammessi", n)
	}
}

func TestFromHTMLProfondita(t *testing.T) {
	src := strings.Repeat("<blockquote>", 40) + "<p>FONDO</p>" + strings.Repeat("</blockquote>", 40)
	blocks, _ := FromHTML(src)
	if d := depthOf(blocks); d == 0 || d > MaxDepth+1 {
		t.Errorf("annidamento = %d, atteso 1..%d (MaxDepth citazioni + foglia)", d, MaxDepth+1)
	}
	if !strings.Contains(allText(blocks), "FONDO") {
		t.Error("il testo oltre la profondita' deve restare (appiattito)")
	}
	src = strings.Repeat("<ul><li>", 40) + "FONDO" + strings.Repeat("</li></ul>", 40)
	blocks, _ = FromHTML(src)
	if d := depthOf(blocks); d == 0 || d > 2*MaxDepth {
		t.Errorf("elenchi annidati = %d, atteso 1..%d", d, 2*MaxDepth)
	}
	if !strings.Contains(allText(blocks), "FONDO") {
		t.Error("testo perso negli elenchi profondi")
	}
}

func TestFromHTMLSanificaTesto(t *testing.T) {
	blocks, _ := FromHTML("<p>a\x01b\x1b[31mc\x07</p>")
	if len(blocks) == 0 {
		t.Fatal("nessun blocco")
	}
	if got := runsText(blocks[0].Runs); strings.ContainsAny(got, "\x01\x1b\x07") {
		t.Errorf("controlli nel testo: %q", got)
	}
}
