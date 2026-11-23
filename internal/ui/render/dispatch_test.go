package render

import (
	"strings"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/msg/richtext"
)

// Piano 15, Task 14 (lavoro C): ogni Kind arriva al renderer giusto, ora che non
// sono piu segnaposto. Si riconosce dal risultato visibile, non dal tipo.
func TestDispatchInstradaOgniKind(t *testing.T) {
	c := testCtx(t, 40)
	runs := []richtext.Run{{Text: "ciao", Href: "https://a.example/"}}
	cases := []struct {
		name string
		b    richtext.Block
		want func(Out) bool
	}{
		{"paragraph", richtext.Block{Kind: richtext.KindParagraph, Runs: runs},
			func(o Out) bool { return o.Zones[0][0].Kind == ZoneLink && plain(o.Lines[0]) == "ciao" }},
		{"pre", richtext.Block{Kind: richtext.KindPre, Runs: []richtext.Run{{Text: "a\n b"}}},
			func(o Out) bool { return len(o.Lines) == 2 && plain(o.Lines[1]) == " b" }},
		{"quote", richtext.Block{Kind: richtext.KindQuote, Children: []richtext.Block{{Runs: runs}}},
			func(o Out) bool { return strings.HasPrefix(plain(o.Lines[0]), "▎ ciao") }},
		{"list", richtext.Block{Kind: richtext.KindList, Ordered: true, Start: 3, Children: []richtext.Block{{Runs: runs}}},
			func(o Out) bool { return strings.HasPrefix(plain(o.Lines[0]), "3. ciao") }},
		{"table", richtext.Block{Kind: richtext.KindTable, Rows: [][]richtext.Cell{{{Runs: runs}}}},
			func(o Out) bool {
				return strings.Contains(joinPlain(o), "ciao") && strings.Contains(joinPlain(o), "┌")
			}},
		{"button", richtext.Block{Kind: richtext.KindButton, Runs: runs, Host: "a.example"},
			func(o Out) bool { return len(o.Lines) == 3 && o.Zones[0][0].Kind == ZoneButton }},
		{"image", richtext.Block{Kind: richtext.KindImage, Alt: "foto"},
			func(o Out) bool { return plain(o.Lines[0]) == "[img: foto]" }},
		{"rule", richtext.Block{Kind: richtext.KindRule},
			func(o Out) bool { return plain(o.Lines[0]) == strings.Repeat("─", 40) }},
	}
	for _, tc := range cases {
		o := Dispatch(tc.b, c)
		valid(t, o)
		if len(o.Lines) == 0 || len(o.Zones) == 0 || !tc.want(o) {
			t.Errorf("%s: instradamento sbagliato: %q %+v", tc.name, o.Lines, o.Zones)
		}
	}
}
