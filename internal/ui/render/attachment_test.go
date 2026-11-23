package render

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestAttachmentChip(t *testing.T) {
	o := Attachment("fattura.pdf", "application/pdf", 1536, 2, testCtx(t, 60))
	need(t, o, 1)
	want := "▣ fattura.pdf · application/pdf · 1,5 KB"
	if p := plain(o.Lines[0]); p != want {
		t.Fatalf("chip = %q, want %q", p, want)
	}
	zs := o.Zones[0]
	if len(zs) != 1 {
		t.Fatalf("zone = %+v, want 1", zs)
	}
	z := zs[0]
	if z.Kind != ZoneAttachment || z.Index != 2 || z.X0 != 0 || z.X1 != lipgloss.Width(want) {
		t.Errorf("zona = %+v, want ZoneAttachment Index 2 [0,%d)", z, lipgloss.Width(want))
	}
}

func TestAttachmentDimensioni(t *testing.T) {
	cases := []struct {
		size int64
		want string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1024, "1,0 KB"},
		{1536, "1,5 KB"},
		{5 << 20, "5,0 MB"},
	}
	for _, c := range cases {
		o := Attachment("a.txt", "text/plain", c.size, 0, testCtx(t, 80))
		need(t, o, 1)
		if p := plain(o.Lines[0]); !strings.HasSuffix(p, " · "+c.want) {
			t.Errorf("size %d: chip = %q, want suffisso %q", c.size, p, c.want)
		}
	}
}

func TestAttachmentNomeTroncatoA40(t *testing.T) {
	long := strings.Repeat("a", 80) + ".pdf"
	o := Attachment(long, "application/pdf", 10, 0, testCtx(t, 200))
	need(t, o, 1)
	p := plain(o.Lines[0])
	name := strings.TrimSuffix(strings.TrimPrefix(p, "▣ "), " · application/pdf · 10 B")
	if w := lipgloss.Width(name); w > 40 || !strings.HasSuffix(name, "…") {
		t.Errorf("nome = %q (%d colonne), want ≤ 40 e con …", name, w)
	}
}

func TestAttachmentNomeControlliRimossi(t *testing.T) {
	// bidi U+202E (nasconde l'estensione reale), NUL, BEL, ESC.
	o := Attachment("rel\u202Eexe.pdf\x00\x07\x1b", "application/pdf", 10, 0, testCtx(t, 80))
	need(t, o, 1)
	if strings.ContainsAny(o.Lines[0], "\u202E\x00\x07") || strings.Contains(plain(o.Lines[0]), "\x1b") {
		t.Errorf("controlli residui: %q", o.Lines[0])
	}
	if p := plain(o.Lines[0]); !strings.HasPrefix(p, "▣ relexe.pdf · ") {
		t.Errorf("chip = %q, want nome sanificato", p)
	}
}

func TestAttachmentLarghezzaStretta(t *testing.T) {
	o := Attachment("fattura-molto-lunga-2026.pdf", "application/pdf", 5<<20, 1, testCtx(t, 20))
	need(t, o, 1)
	if w := lipgloss.Width(o.Lines[0]); w > 20 {
		t.Errorf("chip largo %d colonne, want ≤ 20: %q", w, plain(o.Lines[0]))
	}
	if !strings.Contains(plain(o.Lines[0]), "…") {
		t.Errorf("chip = %q, want troncato con …", plain(o.Lines[0]))
	}
	if z := o.Zones[0][0]; z.X1 != lipgloss.Width(o.Lines[0]) {
		t.Errorf("zona X1 = %d, want %d", z.X1, lipgloss.Width(o.Lines[0]))
	}
}
