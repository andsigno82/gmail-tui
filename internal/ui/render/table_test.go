package render

import (
	"regexp"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/andsigno82/gmail-tui/internal/msg/richtext"
)

// ansiRE toglie SGR e OSC 8 dalle righe rese (Piano 15, Task 8).
var ansiRE = regexp.MustCompile(`\x1b\[[0-9;:]*[A-Za-z]|\x1b\]8;[^\x1b]*\x1b\\`)

// plain ritorna la riga senza sequenze di escape.
func plain(s string) string { return ansiRE.ReplaceAllString(s, "") }

// cut ritorna le celle [x0,x1) della riga resa, senza escape: e il testo che
// l'utente vede sotto la zona (Piano 15, Task 8).
func cut(line string, x0, x1 int) string {
	var b strings.Builder
	col := 0
	for _, r := range plain(line) {
		w := lipgloss.Width(string(r))
		if col >= x0 && col+w <= x1 {
			b.WriteRune(r)
		}
		col += w
	}
	return b.String()
}

// zoneTexts elenca "testo|url" di ogni zona, riga per riga: il testo e quello
// realmente visibile alle colonne della zona. Verifica anche l'invariante di Out.
func zoneTexts(t *testing.T, o Out) []string {
	t.Helper()
	valid(t, o)
	var got []string
	for i, zs := range o.Zones {
		for _, z := range zs {
			if z.X0 >= z.X1 {
				t.Errorf("riga %d: zona vuota %+v", i, z)
			}
			got = append(got, cut(o.Lines[i], z.X0, z.X1)+"|"+z.URL)
		}
	}
	return got
}

// zoneJoined ricompone il testo visibile delle zone con url (spazio tra le
// porzioni): un link spezzato a capo deve ridare il testo originale.
func zoneJoined(t *testing.T, o Out, url string) string {
	t.Helper()
	var parts []string
	for _, g := range zoneTexts(t, o) {
		if s, ok := strings.CutSuffix(g, "|"+url); ok {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " ")
}

// maxWidth e la larghezza in colonne della riga piu larga.
func maxWidth(o Out) int {
	w := 0
	for _, l := range o.Lines {
		w = max(w, lipgloss.Width(l))
	}
	return w
}

func joinPlain(o Out) string {
	ls := make([]string, len(o.Lines))
	for i, l := range o.Lines {
		ls[i] = plain(l)
	}
	return strings.Join(ls, "\n")
}

func txt(s string) []richtext.Run { return []richtext.Run{{Text: s}} }

func lnk(text, href string) richtext.Run { return richtext.Run{Text: text, Href: href} }

func hdrRow(texts ...string) []richtext.Cell {
	r := make([]richtext.Cell, len(texts))
	for i, s := range texts {
		r[i] = richtext.Cell{Runs: txt(s), Header: true}
	}
	return r
}

func bodyRow(texts ...string) []richtext.Cell {
	r := make([]richtext.Cell, len(texts))
	for i, s := range texts {
		if s != "" {
			r[i] = richtext.Cell{Runs: txt(s)}
		}
	}
	return r
}

func tbl(rows ...[]richtext.Cell) richtext.Block {
	return richtext.Block{Kind: richtext.KindTable, Rows: rows}
}

// rectangular verifica che tutte le righe abbiano la stessa larghezza e non
// superino max.
func rectangular(t *testing.T, o Out, maxW int) {
	t.Helper()
	if len(o.Lines) == 0 {
		t.Fatal("nessuna riga")
	}
	w := lipgloss.Width(o.Lines[0])
	if w > maxW {
		t.Errorf("larghezza %d > %d:\n%s", w, maxW, joinPlain(o))
	}
	for i, l := range o.Lines {
		if lipgloss.Width(l) != w {
			t.Errorf("riga %d non rettangolare (%d != %d):\n%s", i, lipgloss.Width(l), w, joinPlain(o))
		}
	}
}

// TestTableEntra (Piano 15, Task 8): una tabella che entra ha i bordi, e
// rettangolare e non supera la larghezza.
func TestTableEntra(t *testing.T) {
	b := tbl(hdrRow("Nome", "Eta"), bodyRow("Mario", "42"), bodyRow("Anna", "7"))
	o := Table(b, testCtx(t, 40))
	valid(t, o)
	s := joinPlain(o)
	for _, want := range []string{"┌", "│", "└", "├", "Nome", "Eta", "Mario", "42", "Anna"} {
		if !strings.Contains(s, want) {
			t.Errorf("manca %q in:\n%s", want, s)
		}
	}
	rectangular(t, o, 40)
}

// TestTableSogliaRicaduta (Piano 15, Task 8, D15-T8): somma delle larghezze minime
// (parola piu lunga, intestazione intera) + padding + bordi: a 15 colonne entra
// (con a capo), a 14 ricade su testo.
func TestTableSogliaRicaduta(t *testing.T) {
	b := tbl(hdrRow("Nome", "Eta"), bodyRow("Mario Rossi", "42"))
	o := Table(b, testCtx(t, 15))
	valid(t, o)
	s := joinPlain(o)
	if !strings.Contains(s, "│") {
		t.Fatalf("a 15 colonne la tabella deve entrare:\n%s", s)
	}
	rectangular(t, o, 15)
	var mario, rossi bool
	for _, l := range o.Lines {
		p := plain(l)
		mario = mario || strings.Contains(p, "Mario") && !strings.Contains(p, "Rossi")
		rossi = rossi || strings.Contains(p, "Rossi") && !strings.Contains(p, "Mario")
	}
	if !mario || !rossi {
		t.Errorf("la cella doveva andare a capo:\n%s", s)
	}

	o = Table(b, testCtx(t, 14))
	valid(t, o)
	s = joinPlain(o)
	if strings.ContainsAny(s, "│┌┐└┘├┤┬┴┼─") {
		t.Errorf("a 14 colonne ricade su testo, niente bordi:\n%s", s)
	}
	for _, want := range []string{"Nome: Mario\nRossi", "Eta: 42"} {
		if !strings.Contains(s, want) {
			t.Errorf("manca %q:\n%s", want, s)
		}
	}
	if maxWidth(o) > 14 {
		t.Errorf("la ricaduta supera 14: %d", maxWidth(o))
	}
}

// TestTableRicadutaSenzaIntestazione: senza riga di intestazione la ricaduta
// mostra solo i valori.
func TestTableRicadutaSenzaIntestazione(t *testing.T) {
	b := tbl(bodyRow("alfa beta gamma", "delta epsilon"))
	o := Table(b, testCtx(t, 10))
	valid(t, o)
	s := joinPlain(o)
	if strings.ContainsAny(s, "│┌") {
		t.Errorf("attesa ricaduta su testo:\n%s", s)
	}
	for _, want := range []string{"alfa", "gamma", "epsilon"} {
		if !strings.Contains(s, want) {
			t.Errorf("manca %q:\n%s", want, s)
		}
	}
	if maxWidth(o) > 10 {
		t.Errorf("larghezza %d > 10", maxWidth(o))
	}
}

// TestTableIntestazione: il separatore sotto l'intestazione c'e solo con una
// riga di <th>.
func TestTableIntestazione(t *testing.T) {
	con := Table(tbl(hdrRow("A", "B"), bodyRow("1", "2")), testCtx(t, 30))
	valid(t, con)
	if !strings.Contains(joinPlain(con), "├") {
		t.Errorf("manca il separatore dell'intestazione:\n%s", joinPlain(con))
	}
	senza := Table(tbl(bodyRow("1", "2"), bodyRow("3", "4")), testCtx(t, 30))
	valid(t, senza)
	if strings.Contains(joinPlain(senza), "├") {
		t.Errorf("senza intestazione non ci deve essere il separatore:\n%s", joinPlain(senza))
	}
	if !strings.Contains(joinPlain(senza), "│ 1") {
		t.Errorf("le righe devono comunque avere bordi:\n%s", joinPlain(senza))
	}
}

// TestTableCelleVuoteERigheCorte: celle vuote, colonna tutta vuota e righe piu
// corte delle altre non rompono il riquadro.
func TestTableCelleVuoteERigheCorte(t *testing.T) {
	b := tbl(
		hdrRow("A", "B", "C"),
		bodyRow("x", "", ""),
		bodyRow("y"),
		bodyRow("", "", ""),
	)
	o := Table(b, testCtx(t, 30))
	valid(t, o)
	if !strings.Contains(joinPlain(o), "│") {
		t.Fatalf("attesa tabella:\n%s", joinPlain(o))
	}
	rectangular(t, o, 30)
	if got := strings.Count(joinPlain(o), "┬"); got != 2 {
		t.Errorf("attese 3 colonne (2 incroci), trovati %d", got)
	}
}

// TestTableVuota: nessuna riga o larghezza nulla = Out vuoto valido.
func TestTableVuota(t *testing.T) {
	o := Table(richtext.Block{Kind: richtext.KindTable}, testCtx(t, 20))
	valid(t, o)
	if len(o.Lines) != 0 {
		t.Errorf("tabella senza righe: %q", o.Lines)
	}
	o = Table(tbl(bodyRow("a")), testCtx(t, 0))
	valid(t, o)
	if len(o.Lines) != 0 {
		t.Errorf("larghezza 0: %q", o.Lines)
	}
}

// TestTableCJK: i caratteri larghi (2 colonne) si contano per colonne e si
// spezzano senza spazi; la tabella resta rettangolare e entro la larghezza.
func TestTableCJK(t *testing.T) {
	b := tbl(hdrRow("名前", "説明"), bodyRow("山田太郎", "これは日本語の長い説明です"))
	o := Table(b, testCtx(t, 24))
	valid(t, o)
	s := joinPlain(o)
	if !strings.Contains(s, "│") {
		t.Fatalf("il CJK si spezza tra i caratteri: attesa tabella:\n%s", s)
	}
	rectangular(t, o, 24)
	if !strings.Contains(s, "山田") || !strings.Contains(s, "太郎") {
		t.Errorf("manca il nome (spezzato tra i caratteri):\n%s", s)
	}
}

// TestTableLimiti (D15-T4): oltre 200 righe o 20 colonne ricade su testo.
func TestTableLimiti(t *testing.T) {
	rows := [][]richtext.Cell{hdrRow("A")}
	for range richtext.MaxTableRows + 1 {
		rows = append(rows, bodyRow("x"))
	}
	o := Table(tbl(rows...), testCtx(t, 40))
	valid(t, o)
	if len(o.Lines) == 0 || strings.Contains(joinPlain(o), "│") {
		t.Error("oltre MaxTableRows attesa ricaduta su testo")
	}
	wide := make([]string, richtext.MaxTableCols+1)
	for i := range wide {
		wide[i] = "c"
	}
	o = Table(tbl(bodyRow(wide...)), testCtx(t, 200))
	valid(t, o)
	if len(o.Lines) == 0 || strings.Contains(joinPlain(o), "│") {
		t.Error("oltre MaxTableCols attesa ricaduta su testo")
	}
}

// TestTableZoneLink: la zona del link nella cella sta sulla colonna giusta,
// dopo il bordo sinistro e le colonne precedenti, e il testo visibile sotto la
// zona e quello del link.
func TestTableZoneLink(t *testing.T) {
	cells := []richtext.Cell{
		{Runs: txt("ciao")},
		{Runs: []richtext.Run{{Text: "vai "}, lnk("qui", "https://x.example/a")}},
	}
	b := tbl(hdrRow("A", "B"), cells)
	o := Table(b, testCtx(t, 40))
	if got := zoneTexts(t, o); len(got) != 1 || got[0] != "qui|https://x.example/a" {
		t.Fatalf("zone: %q\n%s", got, joinPlain(o))
	}
	var z Zone
	for _, zs := range o.Zones {
		if len(zs) > 0 {
			z = zs[0]
		}
	}
	if z.X0 < 8 {
		t.Errorf("la zona sta nella seconda colonna, X0=%d", z.X0)
	}
	if z.Kind != ZoneLink || z.ID == "" {
		t.Errorf("zona incompleta: %+v", z)
	}
}

// TestTableZoneLinkACapo: un link che va a capo in una cella stretta ha una
// zona per ogni porzione, con lo stesso id e il testo giusto sotto ciascuna.
func TestTableZoneLinkACapo(t *testing.T) {
	cells := []richtext.Cell{
		{Runs: []richtext.Run{lnk("uno due tre", "https://x.example/l")}},
		{Runs: txt("altra colonna di testo")},
	}
	b := tbl(hdrRow("Primo", "Secondo"), cells)
	o := Table(b, testCtx(t, 24))
	valid(t, o)
	if !strings.Contains(joinPlain(o), "│") {
		t.Fatalf("attesa tabella:\n%s", joinPlain(o))
	}
	if got := zoneTexts(t, o); len(got) < 2 {
		t.Fatalf("il link doveva spezzarsi in piu zone: %q\n%s", got, joinPlain(o))
	}
	if got := zoneJoined(t, o, "https://x.example/l"); got != "uno due tre" {
		t.Errorf("le porzioni non ricompongono il link: %q", got)
	}
	var id string
	for _, zs := range o.Zones {
		for _, z := range zs {
			if id == "" {
				id = z.ID
			}
			if id == "" || z.ID != id {
				t.Errorf("id non coerente: %+v", z)
			}
		}
	}
}

// TestTableZoneRicaduta: anche nella ricaduta su testo le zone puntano al testo.
func TestTableZoneRicaduta(t *testing.T) {
	cells := []richtext.Cell{{Runs: []richtext.Run{{Text: "vedi "}, lnk("sito", "https://x.example/r")}}}
	b := tbl(hdrRow("Colonna molto lunga"), cells)
	o := Table(b, testCtx(t, 12))
	if strings.Contains(joinPlain(o), "│") {
		t.Fatalf("attesa ricaduta:\n%s", joinPlain(o))
	}
	if got := zoneTexts(t, o); len(got) != 1 || got[0] != "sito|https://x.example/r" {
		t.Errorf("zone: %q\n%s", got, joinPlain(o))
	}
}

// TestTableDispatch: Dispatch instrada KindTable a Table.
func TestTableDispatch(t *testing.T) {
	o := Dispatch(tbl(hdrRow("A"), bodyRow("1")), testCtx(t, 20))
	valid(t, o)
	if !strings.Contains(joinPlain(o), "│") {
		t.Errorf("Dispatch non rende la tabella:\n%s", joinPlain(o))
	}
}
