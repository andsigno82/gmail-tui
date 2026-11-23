package render

import (
	"regexp"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/andsigno82/gmail-tui/internal/msg/richtext"
)

// sgrRE e oscRE tolgono stili e sequenze OSC 8 per confrontare il testo visibile
// (Piano 15, Task 7).
var (
	sgrRE = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)
	oscRE = regexp.MustCompile(`\x1b\][^\x1b\x07]*(?:\x1b\\|\x07)`)
)

// plain ritorna il testo visibile di una riga resa.
func plain(s string) string { return oscRE.ReplaceAllString(sgrRE.ReplaceAllString(s, ""), "") }

// need ferma il test se Out non ha n righe (RED senza panico sugli indici).
func need(t *testing.T, o Out, n int) {
	t.Helper()
	valid(t, o)
	if len(o.Lines) != n {
		t.Fatalf("righe = %d, want %d: %q", len(o.Lines), n, o.Lines)
	}
}

func btn(text, href, host string, warn bool) richtext.Block {
	return richtext.Block{
		Kind: richtext.KindButton,
		Runs: []richtext.Run{{Text: text, Href: href}},
		Host: host,
		Warn: warn,
	}
}

func TestButtonRiquadroArrotondato(t *testing.T) {
	o := Button(btn("Apri", "https://amazon.it/x", "amazon.it", false), testCtx(t, 40))
	need(t, o, 3)
	want := []string{"╭──────╮", "│ Apri │ → amazon.it", "╰──────╯"}
	for i, w := range want {
		if got := plain(o.Lines[i]); got != w {
			t.Errorf("riga %d = %q, want %q", i, got, w)
		}
	}
}

func TestButtonZonaSuTutteLeRighe(t *testing.T) {
	o := Button(btn("Apri", "https://amazon.it/x", "amazon.it", false), testCtx(t, 40))
	need(t, o, 3)
	for i, zs := range o.Zones {
		if len(zs) != 1 {
			t.Fatalf("riga %d: zone = %+v, want 1", i, zs)
		}
		z := zs[0]
		if z.Kind != ZoneButton || z.URL != "https://amazon.it/x" || z.X0 != 0 || z.X1 != 8 {
			t.Errorf("riga %d: zona = %+v, want ZoneButton [0,8) con URL", i, z)
		}
		if z.ID == "" || z.ID != o.Zones[0][0].ID {
			t.Errorf("riga %d: id OSC 8 = %q, deve essere comune e non vuoto", i, z.ID)
		}
		// Ogni riga porta l'OSC 8 con lo stesso id e l'URL.
		open := "\x1b]8;id=" + z.ID + ";https://amazon.it/x\x1b\\"
		if !strings.Contains(o.Lines[i], open) {
			t.Errorf("riga %d senza OSC 8 %q: %q", i, open, o.Lines[i])
		}
	}
}

func TestButtonTroncaTestoLungo(t *testing.T) {
	long := strings.Repeat("x", 200)
	o := Button(btn(long, "https://example.org/", "example.org", false), testCtx(t, 20))
	need(t, o, 3)
	for i, l := range o.Lines {
		if w := lipgloss.Width(l); w != 20 {
			t.Errorf("riga %d larga %d colonne, want 20: %q", i, w, plain(l))
		}
	}
	if p := plain(o.Lines[1]); !strings.Contains(p, "…") || strings.Contains(p, "→") {
		t.Errorf("riga testo = %q: want troncata con … e senza host", p)
	}
	if z := o.Zones[1][0]; z.X1 != 20 {
		t.Errorf("zona X1 = %d, want 20", z.X1)
	}
}

func TestButtonSenzaHost(t *testing.T) {
	o := Button(btn("Apri", "https://amazon.it/x", "", false), testCtx(t, 40))
	need(t, o, 3)
	if p := plain(o.Lines[1]); strings.Contains(p, "→") || p != "│ Apri │" {
		t.Errorf("riga = %q, want solo il riquadro", p)
	}
}

func TestButtonTestoSanificato(t *testing.T) {
	// bidi (U+202E), zero-width (U+200B), BEL, tab: il testo resta leggibile.
	o := Button(btn("Cli\u202Ecca\u200Bqui\x07\tsubito", "https://example.org/", "", false), testCtx(t, 60))
	need(t, o, 3)
	if p := plain(o.Lines[1]); !strings.Contains(p, "Cliccaqui subito") {
		t.Errorf("testo = %q, want sanificato \"Cliccaqui subito\"", p)
	}
	if strings.ContainsAny(o.Lines[1], "\u202E\u200B\x07\t") {
		t.Errorf("caratteri di controllo residui: %q", o.Lines[1])
	}
}

func TestButtonWarnRiga(t *testing.T) {
	o := Button(btn("amazon.it", "https://amazon.evil.example/", "amazon.evil.example", true), testCtx(t, 60))
	need(t, o, 4)
	if p := plain(o.Lines[2]); !strings.Contains(p, "⚠") {
		t.Errorf("riga di avviso = %q, want ⚠", p)
	}
	for i, zs := range o.Zones {
		if len(zs) != 1 || !zs[0].Warn || zs[0].URL != "https://amazon.evil.example/" {
			t.Errorf("riga %d: zona = %+v, want Warn con URL", i, zs)
		}
	}
}

func TestButtonURLNonValidoSenzaZona(t *testing.T) {
	o := Button(btn("Apri", "javascript:alert(1)", "", false), testCtx(t, 40))
	need(t, o, 3)
	for i, zs := range o.Zones {
		if zs != nil {
			t.Errorf("riga %d: zona %+v per URL non valido", i, zs)
		}
	}
	for i, l := range o.Lines {
		if strings.Contains(l, "\x1b]8;") {
			t.Errorf("riga %d con OSC 8 per URL non valido: %q", i, l)
		}
	}
}

func TestButtonLarghezzaZeroVuoto(t *testing.T) {
	if o := Button(btn("Apri", "https://example.org/", "", false), testCtx(t, 0)); len(o.Lines) != 0 {
		t.Fatalf("Width 0: %+v, want vuoto", o)
	}
}
