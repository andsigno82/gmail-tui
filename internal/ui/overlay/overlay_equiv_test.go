package overlay

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/andsigno82/gmail-tui/internal/ui/styles"
)

// overlayRef e l'Overlay originale del Piano 15 (compositor sull'intera base): il
// riferimento con cui si verifica che la versione a banda (solo le righe coperte da
// finestra e ombra) produca la stessa griglia di celle.
func overlayRef(base, win string, x, y int, st styles.Styles) string {
	bw, bh := lipgloss.Width(base), lipgloss.Height(base)
	ww, wh := lipgloss.Width(win), lipgloss.Height(win)
	x = min(max(x, 0), max(bw-ww-shadowDX, 0))
	y = min(max(y, 0), max(bh-wh-shadowDY, 0))

	row := strings.Repeat(shadowRune, ww)
	shadow := st.OverlayShadow.UnsetBackground().Render(strings.TrimSuffix(strings.Repeat(row+"\n", wh), "\n"))
	out := lipgloss.NewCompositor(
		lipgloss.NewLayer(base),
		lipgloss.NewLayer(shadow).X(x+shadowDX).Y(y+shadowDY).Z(1),
		lipgloss.NewLayer(win).X(x).Y(y).Z(2),
	).Render()
	if n := strings.Count(out, "\n") + 1; n < bh {
		out += strings.Repeat("\n", bh-n)
	}
	return lipgloss.NewStyle().MaxWidth(bw).MaxHeight(bh).Render(out)
}

// styledBase costruisce una base w x h con righe di stili diversi (sfondi, grassetto,
// righe vuote, una riga stretta), come le liste dell'app.
func styledBase(st styles.Styles, w, h int) string {
	rows := make([]string, h)
	for i := range rows {
		switch i % 5 {
		case 0:
			rows[i] = st.ListSelected.Render(strings.Repeat("s", w))
		case 1:
			rows[i] = st.ListUnread.Render(fmt.Sprintf("%-*s", w, fmt.Sprintf("riga %d non letta", i)))
		case 2:
			rows[i] = st.ListNormal.Render(strings.Repeat("n", w/2)) + st.ListDate.Render(strings.Repeat("d", w-w/2))
		case 3:
			rows[i] = strings.Repeat(" ", w)
		default:
			rows[i] = ""
		}
	}
	return strings.Join(rows, "\n")
}

// attrCell e una cella con lo stato SGR completo (colori e attributi): due stringhe
// che il terminale mostra uguali hanno la stessa griglia, anche se le sequenze SGR
// sono scritte in modo diverso (il compositor riscrive le righe che tocca).
type attrCell struct {
	r      rune
	fg, bg string
	attrs  string
}

var sgrAnyRE = regexp.MustCompile(`^\x1b\[([0-9;:]*)m`)

// gridOf ricostruisce la griglia con lo stato SGR per cella; le celle finali di una
// riga che sono spazi senza stile si tolgono (il compositor le taglia).
func gridOf(s string) [][]attrCell {
	var grid [][]attrCell
	for _, line := range strings.Split(s, "\n") {
		var row []attrCell
		var fg, bg string
		attrs := map[string]bool{}
		for i := 0; i < len(line); {
			if m := sgrAnyRE.FindStringSubmatch(line[i:]); m != nil {
				ps := strings.Split(m[1], ";")
				for j := 0; j < len(ps); j++ {
					switch p := ps[j]; {
					case p == "" || p == "0":
						fg, bg, attrs = "", "", map[string]bool{}
					case (p == "38" || p == "48") && j+4 < len(ps) && ps[j+1] == "2":
						v := strings.Join(ps[j+2:j+5], ";")
						if p == "38" {
							fg = v
						} else {
							bg = v
						}
						j += 4
					case p == "39":
						fg = ""
					case p == "49":
						bg = ""
					case p == "22":
						delete(attrs, "1")
						delete(attrs, "2")
					default:
						attrs[p] = true
					}
				}
				i += len(m[0])
				continue
			}
			for _, r := range line[i:] {
				var keys []string
				for k := range attrs {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				row = append(row, attrCell{r: r, fg: fg, bg: bg, attrs: strings.Join(keys, ",")})
				i += len(string(r))
				break
			}
		}
		for len(row) > 0 && row[len(row)-1] == (attrCell{r: ' '}) {
			row = row[:len(row)-1]
		}
		grid = append(grid, row)
	}
	return grid
}

// firstDiff descrive la prima cella diversa fra due griglie.
func firstDiff(a, b [][]attrCell) string {
	if len(a) != len(b) {
		return fmt.Sprintf("righe %d vs %d", len(a), len(b))
	}
	for y := range a {
		if len(a[y]) != len(b[y]) {
			return fmt.Sprintf("riga %d: celle %d vs %d (%v | %v)", y, len(a[y]), len(b[y]), tail(a[y]), tail(b[y]))
		}
		for x := range a[y] {
			if a[y][x] != b[y][x] {
				return fmt.Sprintf("riga %d col %d: %+v vs %+v", y, x, a[y][x], b[y][x])
			}
		}
	}
	return "uguali"
}

func tail(r []attrCell) []attrCell {
	if len(r) > 3 {
		return r[len(r)-3:]
	}
	return r
}

func TestOverlayABandaEquivaleAlCompositorePieno(t *testing.T) {
	for _, th := range []string{"default", "dracula"} {
		st := stylesPer(t, th)
		for _, bs := range [][2]int{{60, 12}, {120, 35}, {200, 50}, {30, 6}} {
			base := styledBase(st, bs[0], bs[1])
			for _, ws := range [][2]int{{20, 5}, {40, 10}, {bs[0] - 2, bs[1] - 2}, {bs[0] + 5, bs[1] + 3}, {10, 3}} {
				win := Window("contenuto\nsecondo rigo\nterzo", "titolo", st, ws[0], ws[1])
				cx, cy := Center(base, win)
				for _, pos := range [][2]int{{cx, cy}, {0, 0}, {bs[0], bs[1]}, {-3, -2}, {bs[0] - 5, 1}, {1, bs[1] - 2}} {
					want := overlayRef(base, win, pos[0], pos[1], st)
					got := Overlay(base, win, pos[0], pos[1], st)
					gg, wg := gridOf(got), gridOf(want)
					if d := firstDiff(gg, wg); d != "uguali" {
						t.Fatalf("tema %s base %v finestra %v pos %v: griglia diversa dal riferimento: %s", th, bs, ws, pos, d)
					}
					if gl, wl := strings.Count(got, "\n"), strings.Count(want, "\n"); gl != wl {
						t.Fatalf("righe %d, voglio %d", gl+1, wl+1)
					}
				}
			}
		}
	}
}
