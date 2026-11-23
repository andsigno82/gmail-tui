package statusline

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/andsigno82/gmail-tui/internal/theme"
	"github.com/andsigno82/gmail-tui/internal/ui/styles"
)

// stiliPiani restituisce stili senza attributi: Render lascia il testo invariato,
// così i test verificano struttura e larghezza, non colori (Piano 15, Task 13).
func stiliPiani() styles.Styles {
	return styles.Styles{
		StatusSegment: []lipgloss.Style{lipgloss.NewStyle(), lipgloss.NewStyle()},
		StatusSep:     lipgloss.NewStyle(),
	}
}

// stiliTema usa gli stili veri del tema (Task 9 fornisce i valori definitivi).
func stiliTema(t *testing.T) styles.Styles {
	t.Helper()
	th, err := theme.Load(theme.Names()[0])
	if err != nil {
		t.Fatalf("caricamento tema: %v", err)
	}
	return styles.New(th)
}

var segmentiTipici = []Segment{
	{Text: "NORMALE", Kind: SegMode},
	{Text: "Posta in arrivo", Kind: SegFolder},
	{Text: "12 messaggi", Kind: SegStatus},
	{Text: "j/k naviga  invio apri  ? aiuto", Kind: SegHelp},
}

func TestLineLarghezzaEsatta(t *testing.T) {
	for _, w := range []int{20, 40, 80, 120} {
		got := Line(segmentiTipici, w, stiliTema(t))
		if lipgloss.Width(got) != w {
			t.Errorf("larghezza %d: lipgloss.Width = %d (%q)", w, lipgloss.Width(got), got)
		}
	}
}

func TestLineSegmentiInOrdine(t *testing.T) {
	got := Line(segmentiTipici, 120, stiliPiani())
	ultima := -1
	for _, s := range segmentiTipici {
		i := strings.Index(got, s.Text)
		if i < 0 {
			t.Fatalf("segmento %q assente in %q", s.Text, got)
		}
		if i <= ultima {
			t.Errorf("segmento %q fuori ordine in %q", s.Text, got)
		}
		ultima = i
	}
}

func TestLineSenzaGlifiSpeciali(t *testing.T) {
	got := Line(segmentiTipici, 120, stiliPiani())
	for _, r := range got {
		if r >= 0xE000 && r <= 0xF8FF {
			t.Errorf("glifo Powerline (U+%04X) in una barra senza glifi", r)
		}
	}
}

func TestLineTroncaPrimaLHelp(t *testing.T) {
	// Larghezza tale che tutti i segmenti non-help ci stanno, ma non l'help intero.
	w := 10
	for _, s := range segmentiTipici[:3] {
		w += lipgloss.Width(s.Text) + 2 // testo più un margine per lato, senza separatori (stiliPiani)
	}
	got := Line(segmentiTipici, w, stiliPiani())
	if lipgloss.Width(got) != w {
		t.Fatalf("larghezza %d attesa, ottenuta %d", w, lipgloss.Width(got))
	}
	for _, s := range segmentiTipici[:3] {
		if !strings.Contains(got, s.Text) {
			t.Errorf("segmento %q non-help perso col troncamento: %q", s.Text, got)
		}
	}
	if strings.Contains(got, segmentiTipici[3].Text) {
		t.Errorf("l'help intero non si è troncato: %q", got)
	}
	if !strings.Contains(got, "j/k") {
		t.Errorf("l'inizio dell'help dovrebbe restare visibile: %q", got)
	}
}

func TestLineSegmentiVuotiSaltati(t *testing.T) {
	segs := []Segment{{Text: "", Kind: SegMode}, {Text: "Inbox", Kind: SegFolder}, {Text: "", Kind: SegHelp}}
	got := Line(segs, 30, stiliPiani())
	if lipgloss.Width(got) != 30 {
		t.Fatalf("larghezza 30 attesa, ottenuta %d (%q)", lipgloss.Width(got), got)
	}
	want := Line([]Segment{{Text: "Inbox", Kind: SegFolder}}, 30, stiliPiani())
	if got != want {
		t.Errorf("segmenti vuoti non saltati:\n got  %q\n want %q", got, want)
	}
}

func TestLineLarghezzePiccoleSenzaPanic(t *testing.T) {
	for w := -2; w <= 12; w++ {
		got := Line(segmentiTipici, w, stiliTema(t))
		if w <= 0 {
			if got != "" {
				t.Errorf("larghezza %d: attesa stringa vuota, ottenuta %q", w, got)
			}
			continue
		}
		if lipgloss.Width(got) != w {
			t.Errorf("larghezza %d: ottenuta %d (%q)", w, lipgloss.Width(got), got)
		}
	}
	if got := Line(nil, 10, stiliPiani()); lipgloss.Width(got) != 10 {
		t.Errorf("senza segmenti la barra deve riempire la larghezza: %q", got)
	}
	if got := Line(segmentiTipici, 10, styles.Styles{}); lipgloss.Width(got) != 10 {
		t.Errorf("Styles vuoti: larghezza 10 attesa, ottenuta %d", lipgloss.Width(got))
	}
}
