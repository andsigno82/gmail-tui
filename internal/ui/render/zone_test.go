package render

import (
	"reflect"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/andsigno82/gmail-tui/internal/msg/richtext"
	"github.com/andsigno82/gmail-tui/internal/theme"
	"github.com/andsigno82/gmail-tui/internal/ui/styles"
)

// valid verifica l'invariante di Out (Piano 15, Task 3): len(Zones) == len(Lines).
func valid(t *testing.T, o Out) {
	t.Helper()
	if len(o.Zones) != len(o.Lines) {
		t.Fatalf("len(Zones)=%d != len(Lines)=%d", len(o.Zones), len(o.Lines))
	}
}

func testCtx(t *testing.T, w int) Ctx {
	t.Helper()
	th, err := theme.Load("default")
	if err != nil {
		t.Fatalf("tema: %v", err)
	}
	return Ctx{Width: w, St: styles.New(th)}
}

func TestStackConcatenaRigheEZone(t *testing.T) {
	za := Zone{X0: 1, X1: 4, Kind: ZoneLink, URL: "https://a.example/", ID: "gmt1"}
	zb := Zone{X0: 0, X1: 3, Kind: ZoneAttachment, Index: 2}
	a := Out{Lines: []string{"a0", "a1"}, Zones: [][]Zone{nil, {za}}}
	b := Out{Lines: []string{"b0"}, Zones: [][]Zone{{zb}}}

	got := Stack(a, Out{}, b)

	want := Out{Lines: []string{"a0", "a1", "b0"}, Zones: [][]Zone{nil, {za}, {zb}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Stack = %+v, want %+v", got, want)
	}
	valid(t, got)
}

func TestStackSenzaArgomentiEVuoto(t *testing.T) {
	got := Stack()
	if len(got.Lines) != 0 || len(got.Zones) != 0 {
		t.Fatalf("Stack() = %+v, want vuoto", got)
	}
}

// Un Out costruito a mano con meno zone che righe non rompe l'invariante.
func TestStackRiallineaZoneMancanti(t *testing.T) {
	z := Zone{X0: 0, X1: 2, Kind: ZoneButton}
	short := Out{Lines: []string{"x", "y"}, Zones: [][]Zone{{z}}}
	none := Out{Lines: []string{"p"}}

	got := Stack(short, none)

	valid(t, got)
	if len(got.Lines) != 3 || len(got.Zones[0]) != 1 || got.Zones[1] != nil || got.Zones[2] != nil {
		t.Fatalf("Stack riallineato = %+v", got)
	}
	// una zona in eccesso (senza riga) si scarta: non sposta le zone successive.
	extra := Out{Lines: []string{"q"}, Zones: [][]Zone{nil, {z}}}
	got = Stack(extra, Out{Lines: []string{"r"}, Zones: [][]Zone{{z}}})
	valid(t, got)
	if got.Zones[0] != nil || len(got.Zones[1]) != 1 {
		t.Fatalf("zona in eccesso non scartata: %+v", got.Zones)
	}
}

func TestStackNonModificaGliArgomenti(t *testing.T) {
	z := Zone{X0: 1, X1: 3}
	a := Out{Lines: []string{"a"}, Zones: [][]Zone{{z}}}
	snap := Out{Lines: []string{"a"}, Zones: [][]Zone{{z}}}

	r := Stack(a, a)
	r.Lines[0] = "zzz"
	r.Zones[0][0].X0 = 99

	if !reflect.DeepEqual(a, snap) {
		t.Fatalf("Stack ha modificato l'argomento: %+v", a)
	}
}

func TestIndentSpostaZoneEPrefissaRighe(t *testing.T) {
	z1 := Zone{X0: 2, X1: 6, Kind: ZoneLink, URL: "https://a.example/", ID: "gmt1", Warn: true}
	z2 := Zone{X0: 8, X1: 10, Kind: ZoneButton, URL: "https://b.example/"}
	o := Out{Lines: []string{"ciao mondo", "", "fine"}, Zones: [][]Zone{{z1, z2}, nil, nil}}

	got := Indent(o, 3)

	valid(t, got)
	wantLines := []string{"   ciao mondo", "   ", "   fine"}
	if !reflect.DeepEqual(got.Lines, wantLines) {
		t.Fatalf("Lines = %q, want %q", got.Lines, wantLines)
	}
	if len(got.Zones[0]) != 2 {
		t.Fatalf("zone riga 0 = %+v", got.Zones[0])
	}
	g1, g2 := got.Zones[0][0], got.Zones[0][1]
	if g1.X0 != 5 || g1.X1 != 9 || g2.X0 != 11 || g2.X1 != 13 {
		t.Fatalf("colonne spostate male: %+v %+v", g1, g2)
	}
	// gli altri campi restano.
	g1.X0, g1.X1 = z1.X0, z1.X1
	if g1 != z1 {
		t.Fatalf("campi alterati: %+v vs %+v", g1, z1)
	}
	if got.Zones[1] != nil || got.Zones[2] != nil {
		t.Fatalf("righe senza zone devono restare nil: %+v", got.Zones)
	}
}

// La geometria resta vera sulla riga resa: la zona indentata copre le stesse
// celle del testo, misurate con lipgloss.Width (non len) anche con larghi e OSC 8.
func TestIndentGeometriaSullaRigaResa(t *testing.T) {
	link := "\x1b]8;id=gmt1;https://a.example/\x1b\\" + "世界" + "\x1b]8;;\x1b\\"
	line := "ab " + link
	o := Out{Lines: []string{line}, Zones: [][]Zone{{{X0: 3, X1: 7, Kind: ZoneLink, URL: "https://a.example/"}}}}

	got := Indent(o, 4)

	z := got.Zones[0][0]
	if w := lipgloss.Width(got.Lines[0]); w != 4+7 {
		t.Fatalf("larghezza riga = %d, want 11", w)
	}
	if z.X0 != 7 || z.X1 != 11 || z.X1 != lipgloss.Width(got.Lines[0]) {
		t.Fatalf("zona = [%d,%d), deve finire a fine riga (11)", z.X0, z.X1)
	}
}

func TestIndentNonModificaEZeroOMenoInvariato(t *testing.T) {
	z := Zone{X0: 1, X1: 3}
	o := Out{Lines: []string{"abc"}, Zones: [][]Zone{{z}}}

	g := Indent(o, 2)
	g.Zones[0][0].X0 = 99
	g.Lines[0] = "x"
	if o.Zones[0][0] != z || o.Lines[0] != "abc" {
		t.Fatalf("Indent ha modificato l'argomento: %+v", o)
	}

	for _, pad := range []int{0, -3} {
		g := Indent(o, pad)
		valid(t, g)
		if g.Lines[0] != "abc" || g.Zones[0][0] != z {
			t.Fatalf("pad %d: %+v, want invariato", pad, g)
		}
	}
}

func TestIndentRiallineaZoneMancanti(t *testing.T) {
	g := Indent(Out{Lines: []string{"a", "b"}}, 2)
	valid(t, g)
	if len(g.Lines) != 2 || g.Lines[1] != "  b" {
		t.Fatalf("Indent = %+v", g)
	}
}

// Stack e Indent compongono: una citazione annidata sposta le zone dei figli.
func TestStackDiIndentSpostaSoloLeZoneIndentate(t *testing.T) {
	z := Zone{X0: 0, X1: 3, Kind: ZoneLink}
	in := Out{Lines: []string{"abc"}, Zones: [][]Zone{{z}}}

	got := Stack(in, Indent(in, 2))

	valid(t, got)
	if got.Zones[0][0].X0 != 0 || got.Zones[1][0].X0 != 2 || got.Zones[1][0].X1 != 5 {
		t.Fatalf("zone = %+v", got.Zones)
	}
}

func TestZoneAtIntervalloSemiaperto(t *testing.T) {
	zs := []Zone{
		{X0: 2, X1: 5, Kind: ZoneLink, URL: "u1"},
		{X0: 5, X1: 8, Kind: ZoneButton, URL: "u2"},
	}
	cases := []struct {
		x    int
		url  string
		kind ZoneKind
		ok   bool
	}{
		{1, "", 0, false},
		{2, "u1", ZoneLink, true},
		{4, "u1", ZoneLink, true},
		{5, "u2", ZoneButton, true}, // X1 e escluso, X0 incluso
		{7, "u2", ZoneButton, true},
		{8, "", 0, false},
		{-1, "", 0, false},
	}
	for _, c := range cases {
		z, ok := ZoneAt(zs, c.x)
		if ok != c.ok || z.URL != c.url || (ok && z.Kind != c.kind) {
			t.Errorf("ZoneAt(%d) = %+v,%v; want url=%q kind=%d ok=%v", c.x, z, ok, c.url, c.kind, c.ok)
		}
	}
	if _, ok := ZoneAt(nil, 0); ok {
		t.Error("ZoneAt(nil) deve essere false")
	}
}

// La zona dopo Indent si trova alla colonna indentata, non a quella di origine.
func TestZoneAtDopoIndent(t *testing.T) {
	o := Out{Lines: []string{"abc"}, Zones: [][]Zone{{{X0: 0, X1: 3, Kind: ZoneAttachment, Index: 4}}}}
	g := Indent(o, 5)
	if _, ok := ZoneAt(g.Zones[0], 2); ok {
		t.Error("colonna 2 e nel rientro: nessuna zona")
	}
	z, ok := ZoneAt(g.Zones[0], 6)
	if !ok || z.Kind != ZoneAttachment || z.Index != 4 {
		t.Errorf("colonna 6: %+v,%v", z, ok)
	}
}

func TestDispatchRegolaOrizzontale(t *testing.T) {
	c := testCtx(t, 12)

	got := Dispatch(richtext.Block{Kind: richtext.KindRule}, c)

	valid(t, got)
	if len(got.Lines) != 1 {
		t.Fatalf("righe = %d, want 1", len(got.Lines))
	}
	if w := lipgloss.Width(got.Lines[0]); w != 12 {
		t.Fatalf("larghezza regola = %d, want 12", w)
	}
	if got.Zones[0] != nil {
		t.Fatalf("la regola non e cliccabile: %+v", got.Zones[0])
	}
	if n := len(Dispatch(richtext.Block{Kind: richtext.KindRule}, testCtx(t, 0)).Lines); n != 0 {
		t.Fatalf("Width 0: %d righe, want 0", n)
	}
}

func TestDispatchKindSconosciutoVuoto(t *testing.T) {
	got := Dispatch(richtext.Block{Kind: richtext.Kind(250)}, testCtx(t, 40))
	if len(got.Lines) != 0 || len(got.Zones) != 0 {
		t.Fatalf("Kind sconosciuto = %+v, want vuoto", got)
	}
}

// Per ogni Kind (anche con gli stub dei Task 6-8) il risultato rispetta
// l'invariante len(Zones)==len(Lines): contratto che i renderer devono mantenere.
func TestDispatchInvarianteOgniKind(t *testing.T) {
	c := testCtx(t, 40)
	for k := richtext.KindParagraph; k <= richtext.KindPre; k++ {
		b := richtext.Block{
			Kind: k,
			Runs: []richtext.Run{{Text: "testo "}, {Text: "link", Href: "https://a.example/"}},
			Alt:  "alt",
			Host: "a.example",
			Rows: [][]richtext.Cell{{{Runs: []richtext.Run{{Text: "c"}}}}},
		}
		valid(t, Dispatch(b, c))
	}
}
