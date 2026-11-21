package styles

import (
	"reflect"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"

	"gmail-tui/internal/theme"
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
