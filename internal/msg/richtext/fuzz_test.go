package richtext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

// FuzzFromHTML (Piano 15, Task 4): FromHTML non va mai in panic e rispetta le
// invarianti di sicurezza su qualunque input. In CI gira solo il corpus dei
// seed (go test); l'esplorazione e' manuale: go test -fuzz=FuzzFromHTML.
func FuzzFromHTML(f *testing.F) {
	seeds, _ := filepath.Glob(filepath.Join("testdata", "html", "*.html"))
	for _, p := range seeds {
		if b, err := os.ReadFile(p); err == nil {
			f.Add(string(b))
		}
	}
	for _, s := range []string{
		"", "<", "<p", "</p></div>", "<a href=\"javascript:alert(1)\">x</a>",
		"<table><tr><td><table><tr><td>", "<blockquote><blockquote>", "<ul><li><ol start=-1><li>",
		"<a role=button href=https://e.example>\u202e\u200b</a>", "\x00\xff\xc3<p>\x1b[2J",
		strings.Repeat("<div>", 100), strings.Repeat("<td>", 100),
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		blocks, _ := FromHTML(src)
		checkInvariants(t, blocks, 0)
	})
}

// FuzzFromText (Piano 15, Task 5): come FuzzFromHTML, per il corpo solo testo.
func FuzzFromText(f *testing.F) {
	for _, s := range []string{
		"", ">", ">>>> > >", "- ", "1. ", "99999999999999999999. x",
		"> - a\n>> 1. b\n> c\n\n- d", "https://", "mailto:", "https://e.example/a).,;",
		"\x00\xff\xc3\x1b[2J\r\n\t", "http://exa" + string(rune(0x202e)) + "mple.com/" + string(rune(0x200b)),
		strings.Repeat(">", 100) + " x",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		checkInvariants(t, FromText(src), 0)
	})
}

// checkInvariants: ogni Href passa sanitizeURL, nessun testo contiene
// controlli (tranne \n), nessun annidamento oltre i limiti.
func checkInvariants(t *testing.T, blocks []Block, depth int) {
	t.Helper()
	if depth > 2*MaxDepth {
		t.Fatalf("annidamento %d oltre il limite", depth)
	}
	checkRuns := func(runs []Run) {
		for _, r := range runs {
			if r.Href != "" {
				if got, ok := sanitizeURL(r.Href); !ok || got != r.Href {
					t.Fatalf("href non sanificato: %q", r.Href)
				}
			}
			for _, c := range r.Text {
				if unicode.IsControl(c) && c != '\n' {
					t.Fatalf("controllo nel testo: %q", r.Text)
				}
			}
		}
	}
	for _, b := range blocks {
		checkRuns(b.Runs)
		for _, row := range b.Rows {
			if len(row) > MaxTableCols {
				t.Fatalf("colonne %d oltre il limite", len(row))
			}
			for _, c := range row {
				checkRuns(c.Runs)
			}
		}
		if len(b.Rows) > MaxTableRows {
			t.Fatalf("righe %d oltre il limite", len(b.Rows))
		}
		checkInvariants(t, b.Children, depth+1)
	}
}
