package styles

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"

	"github.com/andsigno82/gmail-tui/internal/theme"
)

// Piano 15, Task 9: stili derivati dei nuovi componenti (bottoni, link, citazioni,
// badge, finestre, barra di stato) e loro contrasto.

// variante restituisce lo slot nella variante dark o light (come Resolve).
func variante(s theme.Slot, dark bool) string {
	d, l := s.Resolve()
	if dark {
		return d
	}
	return l
}

// adattivo estrae il colore adattivo di uno stile, fallisce se non lo è.
func adattivo(t *testing.T, nome string, c color.Color) compat.AdaptiveColor {
	t.Helper()
	a, ok := c.(compat.AdaptiveColor)
	if !ok {
		t.Fatalf("%s: colore non adattivo (%T)", nome, c)
	}
	return a
}

// vicino dice se due colori differiscono al più di tol per canale a 8 bit
// (Blend1D passa da CIELAB: gli estremi possono arrotondare di 1-2).
func vicino(a, b color.Color, tol int) bool {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	d := func(x, y uint32) int {
		v := int(x>>8) - int(y>>8)
		if v < 0 {
			return -v
		}
		return v
	}
	return d(ar, br) <= tol && d(ag, bg) <= tol && d(ab, bb) <= tol
}

// stiliConSfondoP15 sono gli stili del Piano 15 che DEVONO avere uno sfondo
// derivato (e quindi cadere sotto TestContrastoStiliConSfondo).
func stiliConSfondoP15(s Styles) map[string]lipgloss.Style {
	out := map[string]lipgloss.Style{
		"Button": s.Button, "ButtonFocus": s.ButtonFocus, "ChipAttach": s.ChipAttach,
		"ChipImage": s.ChipImage, "TableHeader": s.TableHeader, "FolderCount": s.FolderCount,
		"OverlayShadow": s.OverlayShadow,
	}
	for i, st := range s.BadgeAvatar {
		out[fmt.Sprintf("BadgeAvatar[%d]", i)] = st
	}
	for i, st := range s.StatusSegment {
		out[fmt.Sprintf("StatusSegment[%d]", i)] = st
	}
	return out
}

// Ogni stile nuovo che porta testo su una superficie ha uno sfondo derivato dal
// tema e il testo passa WCAG >= 4.5:1 (dark e light, tutti i temi).
// TestContrastoStiliConSfondo verifica il valore; qui si impone che lo sfondo
// esista (un segnaposto senza sfondo sfuggirebbe a quel test).
func TestStiliPiano15HannoSfondoEContrasto(t *testing.T) {
	for _, name := range theme.Names() {
		t.Run(name, func(t *testing.T) {
			s := New(loadTheme(t, name))
			for fname, st := range stiliConSfondoP15(s) {
				bg := st.GetBackground()
				if _, none := bg.(lipgloss.NoColor); none {
					t.Errorf("%s: manca lo sfondo", fname)
					continue
				}
				bgA := adattivo(t, fname+" bg", bg)
				fgA := adattivo(t, fname+" fg", st.GetForeground())
				if r := contrastRatio(fgA.Dark, bgA.Dark); r < minContrast {
					t.Errorf("%s dark: contrasto %.2f < %.1f", fname, r, minContrast)
				}
				if r := contrastRatio(fgA.Light, bgA.Light); r < minContrast {
					t.Errorf("%s light: contrasto %.2f < %.1f", fname, r, minContrast)
				}
			}
		})
	}
}

// I testi colorati SENZA sfondo (link, host, celle, etichette dell'albero)
// leggono bene sul bg del tema: WCAG >= 4.5:1, dark e light.
func TestTestoPiano15SuBg(t *testing.T) {
	for _, name := range theme.Names() {
		t.Run(name, func(t *testing.T) {
			th := loadTheme(t, name)
			s := New(th)
			bgD, bgL := th.Bg.Resolve()
			for fname, st := range map[string]lipgloss.Style{
				"Link": s.Link, "LinkWarn": s.LinkWarn, "LinkDashed": s.LinkDashed,
				"ButtonHost": s.ButtonHost, "TableCell": s.TableCell, "FolderTree": s.FolderTree,
				"ErrText": s.ErrText,
			} {
				fgA := adattivo(t, fname, st.GetForeground())
				if r := contrastRatio(fgA.Dark, lipgloss.Color(bgD)); r < minContrast {
					t.Errorf("%s dark: contrasto su bg %.2f < %.1f", fname, r, minContrast)
				}
				if r := contrastRatio(fgA.Light, lipgloss.Color(bgL)); r < minContrast {
					t.Errorf("%s light: contrasto su bg %.2f < %.1f", fname, r, minContrast)
				}
			}
		})
	}
}

// D15-07: link tratteggiati; link sospetti ondulati con la sottolineatura nello
// slot error. La sottolineatura dei tratteggiati ha il colore del testo.
func TestSottolineatureLink(t *testing.T) {
	for _, name := range theme.Names() {
		t.Run(name, func(t *testing.T) {
			th := loadTheme(t, name)
			s := New(th)
			if got := s.LinkDashed.GetUnderlineStyle(); got != lipgloss.UnderlineDashed {
				t.Errorf("LinkDashed: sottolineatura %v, voglio tratteggiata", got)
			}
			if got := s.LinkWarn.GetUnderlineStyle(); got != lipgloss.UnderlineCurly {
				t.Errorf("LinkWarn: sottolineatura %v, voglio ondulata", got)
			}
			errA := adattivo(t, "LinkWarn underline", s.LinkWarn.GetUnderlineColor())
			if !sameColor(errA.Dark, lipgloss.Color(variante(th.Error, true))) ||
				!sameColor(errA.Light, lipgloss.Color(variante(th.Error, false))) {
				t.Errorf("LinkWarn: colore di sottolineatura diverso dallo slot error")
			}
			dashA := adattivo(t, "LinkDashed underline", s.LinkDashed.GetUnderlineColor())
			fgA := adattivo(t, "LinkDashed fg", s.LinkDashed.GetForeground())
			if !sameColor(dashA.Dark, fgA.Dark) || !sameColor(dashA.Light, fgA.Light) {
				t.Errorf("LinkDashed: la sottolineatura deve avere il colore del testo")
			}
			// Nel rendering: ondulato = SGR 4:3, tratteggiato = SGR 4:5.
			setDark(t, true)
			if out := s.LinkWarn.Render("x"); !strings.Contains(out, "4:3") {
				t.Errorf("LinkWarn: manca SGR 4:3 (curly) in %q", out)
			}
			if out := s.LinkDashed.Render("x"); !strings.Contains(out, "4:5") {
				t.Errorf("LinkDashed: manca SGR 4:5 (dashed) in %q", out)
			}
		})
	}
}

// D15-03: QuoteBar ha 4 livelli distinti, sfumati da quoted (livello 0) a
// primary (ultimo), ciascuno per variante; ogni barra è visibile sul bg (non è
// testo: soglia WCAG 3:1).
func TestQuoteBarDerivata(t *testing.T) {
	for _, name := range theme.Names() {
		t.Run(name, func(t *testing.T) {
			th := loadTheme(t, name)
			s := New(th)
			if len(s.QuoteBar) != 4 {
				t.Fatalf("QuoteBar: %d livelli, voglio 4", len(s.QuoteBar))
			}
			for _, dark := range []bool{true, false} {
				bg := lipgloss.Color(variante(th.Bg, dark))
				var livelli []color.Color
				for i, st := range s.QuoteBar {
					a := adattivo(t, fmt.Sprintf("QuoteBar[%d]", i), st.GetForeground())
					c := a.Light
					if dark {
						c = a.Dark
					}
					livelli = append(livelli, c)
					if r := contrastRatio(c, bg); r < 3 {
						t.Errorf("QuoteBar[%d] dark=%v: contrasto %.2f < 3 sul bg", i, dark, r)
					}
				}
				if !vicino(livelli[0], lipgloss.Color(variante(th.Quoted, dark)), 2) {
					t.Errorf("dark=%v: il livello 0 non è lo slot quoted", dark)
				}
				if !vicino(livelli[3], lipgloss.Color(variante(th.Primary, dark)), 2) {
					t.Errorf("dark=%v: l'ultimo livello non è lo slot primary", dark)
				}
				for i := 1; i < len(livelli); i++ {
					if sameColor(livelli[i], livelli[i-1]) {
						t.Errorf("dark=%v: livelli %d e %d uguali", dark, i-1, i)
					}
				}
			}
		})
	}
}

// D15-05: il badge delle iniziali usa il palette delle label (stessi slot, stesso
// ordine) come sfondo, in grassetto.
func TestBadgeAvatarComeLabel(t *testing.T) {
	for _, name := range theme.Names() {
		t.Run(name, func(t *testing.T) {
			th := loadTheme(t, name)
			s := New(th)
			slots := labelSlots(th)
			if len(s.BadgeAvatar) != len(slots) {
				t.Fatalf("BadgeAvatar: %d elementi, palette label %d", len(s.BadgeAvatar), len(slots))
			}
			for i, st := range s.BadgeAvatar {
				bgA := adattivo(t, "badge bg", st.GetBackground())
				if !sameColor(bgA.Dark, lipgloss.Color(variante(slots[i], true))) ||
					!sameColor(bgA.Light, lipgloss.Color(variante(slots[i], false))) {
					t.Errorf("BadgeAvatar[%d]: sfondo diverso dallo slot %d del palette", i, i)
				}
				if !st.GetBold() {
					t.Errorf("BadgeAvatar[%d]: non in grassetto", i)
				}
			}
		})
	}
}

// D15-T11: l'ombra è il bg scurito del 35%, per variante; StatusSegment alterna
// selection e border (D15-12).
func TestOmbraESegmenti(t *testing.T) {
	for _, name := range theme.Names() {
		t.Run(name, func(t *testing.T) {
			th := loadTheme(t, name)
			s := New(th)
			sh := adattivo(t, "OverlayShadow bg", s.OverlayShadow.GetBackground())
			if !sameColor(sh.Dark, lipgloss.Darken(lipgloss.Color(variante(th.Bg, true)), 0.35)) ||
				!sameColor(sh.Light, lipgloss.Darken(lipgloss.Color(variante(th.Bg, false)), 0.35)) {
				t.Errorf("OverlayShadow: sfondo diverso da Darken(bg, 0.35)")
			}
			if len(s.StatusSegment) < 2 {
				t.Fatalf("StatusSegment: %d elementi, voglio >= 2", len(s.StatusSegment))
			}
			for i, want := range []theme.Slot{th.Selection, th.Border} {
				a := adattivo(t, "StatusSegment bg", s.StatusSegment[i].GetBackground())
				if !sameColor(a.Dark, lipgloss.Color(variante(want, true))) ||
					!sameColor(a.Light, lipgloss.Color(variante(want, false))) {
					t.Errorf("StatusSegment[%d]: sfondo diverso dallo slot atteso", i)
				}
			}
		})
	}
}

// D15-T12 e nota di coordinamento (Task 12): le sfumature le calcolano i consumatori
// (overlay: primary da OverlayBorder, accent da Spinner). OverlayBorder porta quindi
// solo il primary; nessuno stile di testo ha una sfumatura.
func TestSfumatureSoloSuBordi(t *testing.T) {
	for _, name := range theme.Names() {
		t.Run(name, func(t *testing.T) {
			th := loadTheme(t, name)
			s := New(th)
			ob := adattivo(t, "OverlayBorder", s.OverlayBorder.GetForeground())
			if !sameColor(ob.Dark, lipgloss.Color(variante(th.Primary, true))) ||
				!sameColor(ob.Light, lipgloss.Color(variante(th.Primary, false))) {
				t.Errorf("OverlayBorder: il foreground non è lo slot primary")
			}
			for fname, st := range map[string]lipgloss.Style{
				"Link": s.Link, "LinkWarn": s.LinkWarn, "LinkDashed": s.LinkDashed,
				"Button": s.Button, "ButtonFocus": s.ButtonFocus, "ButtonHost": s.ButtonHost,
				"ChipAttach": s.ChipAttach, "ChipImage": s.ChipImage, "TableCell": s.TableCell,
				"TableHeader": s.TableHeader, "FolderCount": s.FolderCount,
			} {
				if n := len(st.GetBorderForegroundBlend()); n != 0 {
					t.Errorf("%s: sfumatura (%d stop) su stile di testo", fname, n)
				}
			}
		})
	}
}

// Nota di coordinamento (Task 13): la statusline calcola le larghezze assumendo
// segmenti e separatore senza padding né margini.
func TestStatusSenzaPaddingNeMargini(t *testing.T) {
	s := New(loadTheme(t, "default"))
	for i, st := range append(append([]lipgloss.Style{}, s.StatusSegment...), s.StatusSep) {
		if st.GetHorizontalFrameSize() != 0 || st.GetVerticalFrameSize() != 0 {
			t.Errorf("segmento %d: frame %dx%d, voglio 0", i, st.GetHorizontalFrameSize(), st.GetVerticalFrameSize())
		}
	}
}
