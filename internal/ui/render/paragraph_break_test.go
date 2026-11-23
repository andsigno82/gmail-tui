package render

import (
	"testing"

	"github.com/andsigno82/gmail-tui/internal/msg/richtext"
)

// Piano 15, Task 14: un "\n" nei Runs e un a capo voluto (<br> in HTML, riga del
// testo semplice in FromText): il motore lo rende come riga nuova, non come spazio.
func TestParagraphACapoVoluto(t *testing.T) {
	cases := []struct {
		name string
		runs []richtext.Run
		want []string
	}{
		{"due righe", []richtext.Run{{Text: "A lunedi,\nMario"}}, []string{"A lunedi,", "Mario"}},
		{"riga vuota in mezzo", []richtext.Run{{Text: "a\n\nb"}}, []string{"a", "", "b"}},
		{"a capo finale ignorato", []richtext.Run{{Text: "x\n"}}, []string{"x"}},
		{"a capo dentro il link", []richtext.Run{{Text: "ciao\n"}, {Text: "sito", Href: "https://a.example/"}, {Text: "\nfine"}}, []string{"ciao", "sito", "fine"}},
	}
	for _, tc := range cases {
		o := Paragraph(para(tc.runs...), testCtx(t, 40))
		valid(t, o)
		if len(o.Lines) != len(tc.want) {
			t.Errorf("%s: righe = %q, voglio %q", tc.name, o.Lines, tc.want)
			continue
		}
		for i, w := range tc.want {
			if plain(o.Lines[i]) != w {
				t.Errorf("%s: riga %d = %q, voglio %q", tc.name, i, plain(o.Lines[i]), w)
			}
		}
	}
}
