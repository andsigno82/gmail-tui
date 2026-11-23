package overlay

import (
	"regexp"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/andsigno82/gmail-tui/internal/theme"
	"github.com/andsigno82/gmail-tui/internal/ui/styles"
)

// cell è una cella del terminale ricostruita da una stringa renderizzata:
// rune e colori ("r;g;b", vuoto se assente) di sfondo e primo piano.
type cell struct {
	r      rune
	fg, bg string
}

var (
	sgrRE = regexp.MustCompile(`^\x1b\[([0-9;:]*)m`)
	bgRE  = regexp.MustCompile(`48;2;(\d+;\d+;\d+)`)
	fgRE  = regexp.MustCompile(`38;2;(\d+;\d+;\d+)`)
)

// celle ricostruisce la griglia di celle (righe x colonne) con i colori attivi,
// seguendo le sequenze SGR. Le celle vuote a fine riga mancano: i test confrontano
// solo indici dentro la riga.
func celle(s string) [][]cell {
	var grid [][]cell
	for _, line := range strings.Split(s, "\n") {
		var row []cell
		var fg, bg string
		for i := 0; i < len(line); {
			if m := sgrRE.FindStringSubmatch(line[i:]); m != nil {
				p := m[1]
				if p == "" || p == "0" || strings.HasPrefix(p, "0;") {
					fg, bg = "", ""
				}
				if b := bgRE.FindStringSubmatch(p); b != nil {
					bg = b[1]
				}
				if f := fgRE.FindStringSubmatch(p); f != nil {
					fg = f[1]
				}
				i += len(m[0])
				continue
			}
			for _, r := range line[i:] {
				row = append(row, cell{r: r, fg: fg, bg: bg})
				i += len(string(r))
				break
			}
		}
		grid = append(grid, row)
	}
	return grid
}

func stylesPer(t *testing.T, name string) styles.Styles {
	t.Helper()
	th, err := theme.Load(name)
	if err != nil {
		t.Fatalf("caricamento tema %q: %v", name, err)
	}
	return styles.New(th)
}

// griglia produce una base di w x h celle riempite di 'x'.
func griglia(w, h int) string {
	row := strings.Repeat("x", w)
	rows := make([]string, h)
	for i := range rows {
		rows[i] = row
	}
	return strings.Join(rows, "\n")
}

// Piano 15, Task 12: le dimensioni esterne di Window sono esatte, anche con
// larghezze e altezze piccole (sotto il minimo si arriva a 3x3: bordo + 1 cella).
func TestWindowDimensioni(t *testing.T) {
	casi := []struct{ w, h, wantW, wantH int }{
		{40, 10, 40, 10},
		{20, 5, 20, 5},
		{8, 4, 8, 4},
		{5, 3, 5, 3},
		{3, 3, 3, 3},
		{2, 1, 3, 3},
		{0, 0, 3, 3},
	}
	for _, name := range theme.Names() {
		st := stylesPer(t, name)
		for _, c := range casi {
			out := Window("uno\ndue\ntre\nquattro\ncinque", "Titolo", st, c.w, c.h)
			if gw, gh := lipgloss.Width(out), lipgloss.Height(out); gw != c.wantW || gh != c.wantH {
				t.Errorf("%s Window(w=%d,h=%d) = %dx%d, atteso %dx%d", name, c.w, c.h, gw, gh, c.wantW, c.wantH)
			}
		}
	}
}

// Il titolo compare nella finestra, il contenuto troppo lungo è tagliato in
// altezza (la finestra non cresce) e in larghezza.
func TestWindowTitoloEContenuto(t *testing.T) {
	st := stylesPer(t, theme.Names()[0])
	out := Window("alfa\nbeta\ngamma\ndelta", "Etichette", st, 24, 6)
	plain := sgrRE2.ReplaceAllString(out, "")
	if !strings.Contains(plain, "Etichette") {
		t.Errorf("titolo assente:\n%s", plain)
	}
	if !strings.Contains(plain, "alfa") {
		t.Errorf("contenuto assente:\n%s", plain)
	}
	if strings.Contains(plain, "delta") {
		t.Errorf("contenuto oltre l'altezza non tagliato:\n%s", plain)
	}
	lungo := Window(strings.Repeat("z", 100), "", st, 12, 4)
	if lipgloss.Width(lungo) != 12 || lipgloss.Height(lungo) != 4 {
		t.Errorf("riga lunga sfora: %dx%d", lipgloss.Width(lungo), lipgloss.Height(lungo))
	}
}

var sgrRE2 = regexp.MustCompile(`\x1b\[[0-9;:]*m`)

// D15-13: il bordo ha una sfumatura primary->accent (colori diversi lungo il
// bordo superiore) e nessun colore è fisso: dipende dal tema.
func TestWindowBordoSfumato(t *testing.T) {
	st := stylesPer(t, theme.Names()[0])
	out := Window("x", "", st, 30, 4)
	top := celle(out)[0]
	distinti := map[string]bool{}
	for _, c := range top {
		if c.fg != "" {
			distinti[c.fg] = true
		}
	}
	if len(distinti) < 2 {
		t.Errorf("bordo senza sfumatura: %d colori distinti", len(distinti))
	}
}

// Sfumatura sul titolo: più colori di primo piano distinti sulle lettere.
func TestWindowTitoloSfumato(t *testing.T) {
	st := stylesPer(t, theme.Names()[0])
	out := Window("x", "Titolo lungo abbastanza", st, 40, 5)
	row := celle(out)[1]
	distinti := map[string]bool{}
	for _, c := range row[1:] {
		if c.r != ' ' && c.fg != "" {
			distinti[c.fg] = true
		}
	}
	if len(distinti) < 2 {
		t.Errorf("titolo senza sfumatura: %d colori distinti", len(distinti))
	}
}

// Overlay: le celle sotto la finestra sono coperte (nessuna 'x' dentro il
// rettangolo), quelle fuori restano identiche, l'ombra è visibile a destra
// (+2 colonne) e sotto (+1 riga) e solo fuori dalla finestra.
func TestOverlayCoperturaEOmbra(t *testing.T) {
	st := stylesPer(t, theme.Names()[0])
	const bw, bh = 40, 14
	const x, y, ww, wh = 6, 3, 14, 5
	base := griglia(bw, bh)
	win := Window("ciao", "T", st, ww, wh)
	out := Overlay(base, win, x, y, st)

	if gw, gh := lipgloss.Width(out), lipgloss.Height(out); gw != bw || gh != bh {
		t.Fatalf("dimensioni %dx%d, attese %dx%d", gw, gh, bw, bh)
	}
	g := celle(out)

	inWin := func(cx, cy int) bool { return cx >= x && cx < x+ww && cy >= y && cy < y+wh }
	inShadow := func(cx, cy int) bool {
		return !inWin(cx, cy) && cx >= x+2 && cx < x+2+ww && cy >= y+1 && cy < y+1+wh
	}
	var ombraFg string
	for cy := 0; cy < bh; cy++ {
		for cx := 0; cx < bw; cx++ {
			c := g[cy][cx]
			switch {
			case inWin(cx, cy):
				if c.r == 'x' {
					t.Errorf("cella (%d,%d) sotto la finestra non coperta", cx, cy)
				}
			case inShadow(cx, cy):
				// Retino (richiesta di Andrea, 2026-11-23): ░ nel colore del bordo, senza sfondo.
				if c.r != '░' || c.fg == "" || c.bg != "" {
					t.Errorf("ombra assente in (%d,%d): rune=%q fg=%q bg=%q", cx, cy, c.r, c.fg, c.bg)
				}
				if ombraFg == "" {
					ombraFg = c.fg
				}
				if c.fg != ombraFg {
					t.Errorf("ombra non uniforme in (%d,%d)", cx, cy)
				}
			default:
				if c.r != 'x' || c.bg != "" {
					t.Errorf("cella (%d,%d) fuori da finestra e ombra alterata: %q bg=%q", cx, cy, c.r, c.bg)
				}
			}
		}
	}
	// La finestra sta sopra l'ombra: nessuna cella della finestra è un retino.
	for cy := y; cy < y+wh; cy++ {
		for cx := x; cx < x+ww; cx++ {
			if g[cy][cx].r == '░' {
				t.Errorf("cella (%d,%d) della finestra mostra il retino dell'ombra", cx, cy)
			}
		}
	}
}

// L'ombra deriva dal tema: il retino ░ ha il primo piano dello stile OverlayShadow
// (slot border, Task 9) e nessuno sfondo: il contenuto sotto non viene scurito.
func TestOmbraDerivataDalTema(t *testing.T) {
	st := stylesPer(t, theme.Names()[0])
	st.OverlayShadow = lipgloss.NewStyle().Foreground(lipgloss.Color("#8090a0"))
	out := Overlay(griglia(30, 10), Window("a", "", st, 8, 4), 2, 2, st)
	ombra := celle(out)[3][10] // dentro l'ombra: colonna 2+8 = 10, riga 3
	if ombra.r != '░' || ombra.fg != "128;144;160" || ombra.bg != "" {
		t.Errorf("ombra = %+v, attesa ░ con fg 128;144;160 e senza sfondo", ombra)
	}
}

// Center posiziona la finestra al centro della base.
func TestCenter(t *testing.T) {
	base := griglia(41, 15)
	x, y := Center(base, strings.Repeat("w", 11)+"\n"+strings.Repeat("w", 11)+"\n"+strings.Repeat("w", 11))
	if x != 15 || y != 6 {
		t.Errorf("Center = (%d,%d), atteso (15,6)", x, y)
	}
	// Finestra più grande della base: mai coordinate negative.
	x, y = Center(griglia(5, 3), griglia(20, 10))
	if x != 0 || y != 0 {
		t.Errorf("Center con finestra più grande = (%d,%d), atteso (0,0)", x, y)
	}
}

// Posizioni al bordo o fuori: la finestra e l'ombra restano dentro la base e le
// dimensioni dell'uscita non cambiano; finestra più grande della base: tagliata.
func TestOverlayClampEDimensioni(t *testing.T) {
	st := stylesPer(t, theme.Names()[0])
	const bw, bh = 30, 10
	base := griglia(bw, bh)
	win := Window("c", "T", st, 10, 4)
	for _, p := range [][2]int{{0, 0}, {bw - 10, bh - 4}, {bw, bh}, {-5, -5}, {100, 100}} {
		out := Overlay(base, win, p[0], p[1], st)
		if gw, gh := lipgloss.Width(out), lipgloss.Height(out); gw != bw || gh != bh {
			t.Errorf("pos %v: %dx%d, attese %dx%d", p, gw, gh, bw, bh)
		}
	}
	// Posizione fuori: la finestra viene riportata dentro e l'ombra resta visibile.
	out := Overlay(base, win, 100, 100, st)
	g := celle(out)
	if g[bh-1][bw-1].r != '░' {
		t.Errorf("ombra non riportata dentro la base: ultima cella %+v", g[bh-1][bw-1])
	}

	grande := Window("c", "T", st, 50, 20)
	out = Overlay(base, grande, 0, 0, st)
	if gw, gh := lipgloss.Width(out), lipgloss.Height(out); gw != bw || gh != bh {
		t.Errorf("finestra grande: %dx%d, attese %dx%d", gw, gh, bw, bh)
	}
}

// Base con righe stilizzate: fuori dalla finestra gli stili restano.
func TestOverlayPreservaStiliBase(t *testing.T) {
	st := stylesPer(t, theme.Names()[0])
	riga := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000")).Render(strings.Repeat("x", 30))
	base := strings.Repeat(riga+"\n", 9) + riga
	out := Overlay(base, Window("c", "", st, 10, 4), 10, 3, st)
	g := celle(out)
	if c := g[0][0]; c.r != 'x' || c.fg != "255;0;0" {
		t.Errorf("stile base perso: %+v", c)
	}
}
