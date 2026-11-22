package styles

import (
	"image/color"
	"math"
	"reflect"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"

	"github.com/andsigno82/gmail-tui/internal/theme"
)

func loadTheme(t *testing.T, name string) theme.Theme {
	t.Helper()
	th, err := theme.Load(name)
	if err != nil {
		t.Fatalf("caricamento tema %q: %v", name, err)
	}
	return th
}

// styledFields estrae per nome tutti i campi lipgloss.Style di Styles.
func styledFields(s Styles) map[string]lipgloss.Style {
	out := make(map[string]lipgloss.Style)
	v := reflect.ValueOf(s)
	typ := v.Type()
	for i := 0; i < v.NumField(); i++ {
		if st, ok := v.Field(i).Interface().(lipgloss.Style); ok {
			out[typ.Field(i).Name] = st
		}
	}
	return out
}

// setDark fissa la variante dark/light per la durata del test (compat usa un
// globale: nel contesto di test non c'è TTY, il default dipende da NO_COLOR).
func setDark(t *testing.T, dark bool) {
	t.Helper()
	orig := compat.HasDarkBackground
	compat.HasDarkBackground = dark
	t.Cleanup(func() { compat.HasDarkBackground = orig })
}

// TestNewTuttiITemi: New non deve pannicare per nessun tema embedded e ogni
// campo deve produrre codici ANSI (nessuno stile lasciato a zero → ogni stile
// deriva dal tema).
func TestNewTuttiITemi(t *testing.T) {
	names := theme.Names()
	if len(names) < 12 {
		t.Fatalf("attesi almeno 12 temi, trovati %d: %v", len(names), names)
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			s := New(loadTheme(t, name))
			for fname, st := range styledFields(s) {
				out := st.Render("x")
				if !strings.Contains(out, "\x1b[") {
					t.Errorf("campo %s: rendering senza ANSI (stile non derivato dal tema): %q", fname, out)
				}
			}
		})
	}
}

// TestColoriDalTema: il rendering riflette l'hex dello slot del tema
// (dracula starred dark = #ffb86c → SGR truecolor 38;2;255;184;108).
func TestColoriDalTema(t *testing.T) {
	setDark(t, true)
	s := New(loadTheme(t, "dracula"))
	out := s.ListStarred.Render("★")
	const want = "38;2;255;184;108"
	if !strings.Contains(out, want) {
		t.Fatalf("ListStarred(dracula): atteso SGR %q dentro %q", want, out)
	}
}

// TestVarianteLight: con background light si usa la variante light dello slot
// (default starred light = #bc4c00 → SGR truecolor 38;2;188;76;0).
func TestVarianteLight(t *testing.T) {
	setDark(t, false)
	s := New(loadTheme(t, "default"))
	out := s.ListStarred.Render("★")
	const want = "38;2;188;76;0"
	if !strings.Contains(out, want) {
		t.Fatalf("ListStarred(default, light): atteso SGR %q dentro %q", want, out)
	}
}

// TestTemiDifferiscono: lo stesso testo con due temi produce ANSI diversi —
// i colori arrivano dal tema, non da costanti.
func TestTemiDifferiscono(t *testing.T) {
	setDark(t, true)
	dracula := New(loadTheme(t, "dracula")).StatusBar.Render(" INBOX 42 ")
	onelight := New(loadTheme(t, "one-light")).StatusBar.Render(" INBOX 42 ")
	if dracula == onelight {
		t.Fatalf("StatusBar identica tra dracula e one-light: %q", dracula)
	}
}

// Piano 7.2, Task 1: contrasto WCAG 2.x del testo sulle barre colorate.

// sameColor confronta due colori sui valori RGBA.
func sameColor(a, b color.Color) bool {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

// TestContrastRatio: rapporto WCAG su coppie note; #rgb equivale a #rrggbb.
func TestContrastRatio(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want float64
	}{
		{"nero/bianco", "#000000", "#ffffff", 21},
		{"nero/bianco #rgb", "#000", "#fff", 21},
		{"stesso colore", "#777777", "#777777", 1},
		{"#rgb uguale a #rrggbb", "#fff", "#ffffff", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ca, cb := lipgloss.Color(c.a), lipgloss.Color(c.b)
			got := contrastRatio(ca, cb)
			if math.Abs(got-c.want) > 0.01 {
				t.Fatalf("contrastRatio(%s, %s) = %.3f, atteso %.3f", c.a, c.b, got, c.want)
			}
			if rev := contrastRatio(cb, ca); math.Abs(rev-got) > 1e-9 {
				t.Fatalf("contrastRatio non simmetrico: %.3f vs %.3f", got, rev)
			}
		})
	}
}

// TestOnColor: fg del tema se leggibile, poi bg del tema, altrimenti nero o
// bianco il migliore. Dark e light sono risolte separatamente.
func TestOnColor(t *testing.T) {
	slot := func(hex string) theme.Slot { return theme.Slot{Dark: hex, Light: hex} }
	cases := []struct {
		name         string
		bg, fg, base theme.Slot
		wantDark     string
		wantLight    string
	}{
		{"fg leggibile", slot("#ffffff"), slot("#000000"), slot("#ffffff"), "#000000", "#000000"},
		{"fg no, base sì", slot("#777777"), slot("#ffffff"), slot("#000000"), "#000000", "#000000"},
		{"nessuno: nero", slot("#777777"), slot("#ffffff"), slot("#808080"), "#000000", "#000000"},
		{"nessuno: bianco", slot("#222222"), slot("#333333"), slot("#444444"), "#ffffff", "#ffffff"},
		{
			"variante dark e light separate",
			theme.Slot{Dark: "#000000", Light: "#ffffff"},
			theme.Slot{Dark: "#ffffff"},
			theme.Slot{Light: "#000000"},
			"#ffffff", "#000000",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := onColor(c.bg, c.fg, c.base).(compat.AdaptiveColor)
			if !ok {
				t.Fatal("onColor non restituisce compat.AdaptiveColor")
			}
			if !sameColor(got.Dark, lipgloss.Color(c.wantDark)) {
				t.Errorf("variante dark: atteso %s", c.wantDark)
			}
			if !sameColor(got.Light, lipgloss.Color(c.wantLight)) {
				t.Errorf("variante light: atteso %s", c.wantLight)
			}
		})
	}
}

// TestContrastoStiliConSfondo: per ogni tema embedded e ogni stile con
// Background, il testo ha contrasto >= 4.5 in dark e in light. I colori sono
// letti dagli stili costruiti da New (non da una lista a mano): uno stile
// nuovo con sfondo è coperto automaticamente.
func TestContrastoStiliConSfondo(t *testing.T) {
	const minContrast = 4.5
	for _, name := range theme.Names() {
		t.Run(name, func(t *testing.T) {
			s := New(loadTheme(t, name))
			coperti := 0
			for fname, st := range styledFields(s) {
				bg := st.GetBackground()
				if _, none := bg.(lipgloss.NoColor); none {
					continue
				}
				coperti++
				bgA, ok := bg.(compat.AdaptiveColor)
				if !ok {
					t.Errorf("campo %s: sfondo non adattivo", fname)
					continue
				}
				fgA, ok := st.GetForeground().(compat.AdaptiveColor)
				if !ok {
					t.Errorf("campo %s: testo non adattivo (colore non calcolato)", fname)
					continue
				}
				if r := contrastRatio(fgA.Dark, bgA.Dark); r < minContrast {
					t.Errorf("campo %s dark: contrasto %.2f < %.1f", fname, r, minContrast)
				}
				if r := contrastRatio(fgA.Light, bgA.Light); r < minContrast {
					t.Errorf("campo %s light: contrasto %.2f < %.1f", fname, r, minContrast)
				}
			}
			if coperti == 0 {
				t.Fatal("nessuno stile con sfondo: test vacuo")
			}
		})
	}
}

// TestSlotContrasta (Piano 8, Task 9, D8-5): tabella sui confini della soglia
// WCAG 4.5:1 del testo su bg, in entrambe le varianti dello slot.
func TestSlotContrasta(t *testing.T) {
	cases := []struct {
		name   string
		fg, bg theme.Slot
		want   bool
	}{
		{"nero su bianco", theme.Slot{Dark: "#000000", Light: "#000000"}, theme.Slot{Dark: "#ffffff", Light: "#ffffff"}, true},
		{"#767676 su bianco (4.54:1)", theme.Slot{Dark: "#767676", Light: "#767676"}, theme.Slot{Dark: "#ffffff", Light: "#ffffff"}, true},
		{"#777777 su bianco (4.48:1)", theme.Slot{Dark: "#777777", Light: "#777777"}, theme.Slot{Dark: "#ffffff", Light: "#ffffff"}, false},
		{"variante dark sotto soglia", theme.Slot{Dark: "#222222", Light: "#000000"}, theme.Slot{Dark: "#111111", Light: "#ffffff"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := slotContrasta(c.fg, c.bg); got != c.want {
				t.Errorf("slotContrasta = %v, voglio %v", got, c.want)
			}
		})
	}
}

// TestEtichetteWCAG: per ogni tema embedded il palette delle etichette contiene
// solo slot con contrasto >= 4.5:1 sul bg in entrambe le varianti, e nessun
// candidato valido viene scartato. Il palette non è mai vuoto.
func TestEtichetteWCAG(t *testing.T) {
	for _, name := range theme.Names() {
		t.Run(name, func(t *testing.T) {
			th := loadTheme(t, name)
			palette := labelSlots(th)
			if len(palette) == 0 {
				t.Fatalf("palette etichette vuoto")
			}
			inPalette := make(map[theme.Slot]bool, len(palette))
			for _, s := range palette {
				inPalette[s] = true
				if !slotContrasta(s, th.Bg) {
					t.Errorf("slot %+v nel palette sotto 4.5:1 su bg", s)
				}
			}
			for _, cand := range labelCandidates(th) {
				if want := slotContrasta(cand, th.Bg); want != inPalette[cand] {
					t.Errorf("candidato %+v: nel palette=%v, contrasto ok=%v", cand, inPalette[cand], want)
				}
			}
		})
	}
}

// TestLabelStyleDeterministico: la stessa etichetta ha sempre lo stesso colore,
// e il colore viene dal palette (nessun colore fuori dal tema).
func TestLabelStyleDeterministico(t *testing.T) {
	s := New(loadTheme(t, "default"))
	if len(s.Labels) == 0 {
		t.Fatal("Labels vuoto")
	}
	a := s.LabelStyle("lavoro").Render("x")
	b := s.LabelStyle("lavoro").Render("x")
	if a != b {
		t.Errorf("stessa etichetta, colori diversi: %q vs %q", a, b)
	}
	if !strings.Contains(a, "\x1b[") {
		t.Errorf("LabelStyle senza ANSI: %q", a)
	}
}
