package render

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/andsigno82/gmail-tui/internal/msg/richtext"
)

func TestImageChip(t *testing.T) {
	o := Image("logo", testCtx(t, 80))
	valid(t, o)
	if len(o.Lines) != 1 || plain(o.Lines[0]) != "[img: logo]" {
		t.Errorf("righe = %q", o.Lines)
	}
}

func TestImageNoZones(t *testing.T) {
	for _, alt := range []string{"logo", "", strings.Repeat("x", 300)} {
		o := Image(alt, testCtx(t, 80))
		valid(t, o)
		if len(o.Lines) == 0 {
			t.Fatalf("alt %q: nessuna riga", alt)
		}
		for i, zs := range o.Zones {
			if len(zs) != 0 {
				t.Errorf("alt %q: zone sulla riga %d: %+v", alt, i, zs)
			}
		}
		if strings.Contains(strings.Join(o.Lines, ""), "]8;") {
			t.Errorf("alt %q: OSC 8 in un'immagine", alt)
		}
	}
}

func TestImageEmptyAlt(t *testing.T) {
	for _, alt := range []string{"", "   ", "\n\t", "\x1b\x07", "\u200b"} {
		o := Image(alt, testCtx(t, 80))
		valid(t, o)
		if len(o.Lines) != 1 || plain(o.Lines[0]) != "[img]" {
			t.Errorf("alt %q: righe = %q", alt, o.Lines)
		}
	}
}

func TestImageAltTruncatedTo80Columns(t *testing.T) {
	o := Image(strings.Repeat("a", 200), testCtx(t, 200))
	valid(t, o)
	got := plain(o.Lines[0])
	alt := strings.TrimSuffix(strings.TrimPrefix(got, "[img: "), "]")
	if w := lipgloss.Width(alt); w != 80 {
		t.Errorf("alt largo %d, atteso 80", w)
	}
	if !strings.HasSuffix(alt, "\u2026") {
		t.Errorf("alt troncato senza ellissi: %q", alt)
	}
	// Esattamente 80 colonne: nessun taglio.
	exact := strings.Repeat("b", 80)
	if got := plain(Image(exact, testCtx(t, 200)).Lines[0]); got != "[img: "+exact+"]" {
		t.Errorf("alt di 80 colonne alterato: %q", got)
	}
}

func TestImageAltTruncatedWideChars(t *testing.T) {
	o := Image(strings.Repeat("\u65e5", 100), testCtx(t, 200))
	alt := strings.TrimSuffix(strings.TrimPrefix(plain(o.Lines[0]), "[img: "), "]")
	if w := lipgloss.Width(alt); w > 80 || !strings.HasSuffix(alt, "\u2026") {
		t.Errorf("alt largo %d: %q", w, alt)
	}
}

func TestImageFitsContextWidth(t *testing.T) {
	for _, w := range []int{12, 20, 40} {
		o := Image(strings.Repeat("a", 200), testCtx(t, w))
		valid(t, o)
		if len(o.Lines) != 1 {
			t.Fatalf("righe = %d", len(o.Lines))
		}
		if got := lipgloss.Width(o.Lines[0]); got > w {
			t.Errorf("larghezza %d > %d: %q", got, w, plain(o.Lines[0]))
		}
		if !strings.HasPrefix(plain(o.Lines[0]), "[img: ") || !strings.HasSuffix(plain(o.Lines[0]), "]") {
			t.Errorf("chip malformato: %q", plain(o.Lines[0]))
		}
	}
}

func TestImageAltSanitized(t *testing.T) {
	o := Image("a\x1b[31mb\nc\td\u202ee", testCtx(t, 80))
	valid(t, o)
	got := plain(o.Lines[0])
	if got != "[img: a[31mb c de]" {
		t.Errorf("chip = %q", got)
	}
	if strings.Contains(got, "\x1b") || strings.Contains(o.Lines[0], "\x07") {
		t.Errorf("controlli nel chip: %q", o.Lines[0])
	}
}

func TestDispatchImage(t *testing.T) {
	o := Dispatch(richtext.Block{Kind: richtext.KindImage, Alt: "foto"}, testCtx(t, 80))
	valid(t, o)
	if len(o.Lines) != 1 || plain(o.Lines[0]) != "[img: foto]" {
		t.Errorf("righe = %q", o.Lines)
	}
}
