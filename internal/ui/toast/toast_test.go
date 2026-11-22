package toast

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/andsigno82/gmail-tui/internal/theme"
	"github.com/andsigno82/gmail-tui/internal/ui/styles"
)

// newTest costruisce un toast.Model col tema di default (Piano 7.3, Task 1).
func newTest(t *testing.T) Model {
	t.Helper()
	th, err := theme.Load("default")
	if err != nil {
		t.Fatalf("tema: %v", err)
	}
	return New(styles.New(th))
}

// TestPushScadenza: un toast di successo viene rimosso dal suo ExpiredMsg, e
// il Push di un non-errore arma un timer (cmd non nil).
func TestPushScadenza(t *testing.T) {
	m := newTest(t)
	cmd := m.Push(Success, "archiviata")
	if cmd == nil {
		t.Fatal("Push(Success): atteso un timer (cmd non nil)")
	}
	if got := m.Texts(); len(got) != 1 || got[0] != "archiviata" {
		t.Fatalf("Texts dopo Push = %q, atteso [archiviata]", got)
	}
	m.Update(ExpiredMsg{ID: m.items[0].id})
	if got := m.Texts(); len(got) != 0 {
		t.Fatalf("dopo ExpiredMsg restano toast: %q", got)
	}
}

// TestErroreSenzaTimer: un errore non arma alcun timer (resta finché un tasto
// non lo chiude, vedi TestDismissErrors).
func TestErroreSenzaTimer(t *testing.T) {
	m := newTest(t)
	if cmd := m.Push(Error, "invio fallito"); cmd != nil {
		t.Fatal("Push(Error): non deve armare un timer")
	}
	if got := m.Texts(); len(got) != 1 {
		t.Fatalf("errore non in pila: %q", got)
	}
}

// TestPilaMassimoTre: il quarto Push toglie il più vecchio; l'ordine resta
// cronologico.
func TestPilaMassimoTre(t *testing.T) {
	m := newTest(t)
	for _, s := range []string{"a", "b", "c", "d"} {
		m.Push(Info, s)
	}
	want := []string{"b", "c", "d"}
	if got := m.Texts(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("pila = %q, attesa %q", got, want)
	}
}

// TestExpiredStantio: un ExpiredMsg per un toast già rimosso (dal limite della
// pila) non tocca gli altri; un ID sconosciuto è ignorato.
func TestExpiredStantio(t *testing.T) {
	m := newTest(t)
	m.Push(Info, "a") // id 1: sarà tolto dal quarto Push
	for _, s := range []string{"b", "c", "d"} {
		m.Push(Info, s)
	}
	m.Update(ExpiredMsg{ID: 1})
	m.Update(ExpiredMsg{ID: 999})
	want := []string{"b", "c", "d"}
	if got := m.Texts(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("ExpiredMsg stantio ha toccato la pila: %q, attesa %q", got, want)
	}
}

// TestDismissErrors: DismissErrors toglie solo gli errori, gli altri restano.
func TestDismissErrors(t *testing.T) {
	m := newTest(t)
	m.Push(Info, "info")
	m.Push(Error, "err")
	m.Push(Success, "ok") // tre toast: il limite di pila (3) non esclude nulla
	m.DismissErrors()
	want := []string{"info", "ok"}
	if got := m.Texts(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("dopo DismissErrors = %q, attesa %q", got, want)
	}
}

// TestLayersPosizione: il toast sta in alto a destra con margine di 1 colonna
// dal bordo destro e di 1 riga dall'alto; due toast si impilano senza
// sovrapposizione. Verificato a 80 e 140 colonne.
func TestLayersPosizione(t *testing.T) {
	for _, width := range []int{80, 140} {
		m := newTest(t)
		m.Push(Success, "prima")
		layers := m.Layers(width)
		if len(layers) != 1 {
			t.Fatalf("width %d: attesi 1 layer, trovati %d", width, len(layers))
		}
		l := layers[0]
		if got := l.GetX() + l.Width(); got != width-1 {
			t.Errorf("width %d: bordo destro del toast a %d, atteso %d", width, got, width-1)
		}
		if l.GetY() != 1 {
			t.Errorf("width %d: Y = %d, atteso 1", width, l.GetY())
		}
		if l.GetZ() <= 0 {
			t.Errorf("width %d: Z = %d, atteso > 0 (sopra il corpo)", width, l.GetZ())
		}

		m.Push(Error, "secondo")
		layers = m.Layers(width)
		if len(layers) != 2 {
			t.Fatalf("width %d: attesi 2 layer, trovati %d", width, len(layers))
		}
		if want := layers[0].GetY() + layers[0].Height(); layers[1].GetY() != want {
			t.Errorf("width %d: secondo toast a Y=%d, atteso %d (impilati)", width, layers[1].GetY(), want)
		}
	}
}

// TestTestoLungoSpezzato: un testo lungo va a capo entro ~40% dello schermo
// (80 colonne → box al più 32 colonne) e nessuna riga supera quella larghezza.
func TestTestoLungoSpezzato(t *testing.T) {
	m := newTest(t)
	long := strings.Repeat("oggetto molto lungo di una mail ", 8)
	m.Push(Info, long)
	layers := m.Layers(80)
	if len(layers) != 1 {
		t.Fatalf("attesi 1 layer, trovati %d", len(layers))
	}
	l := layers[0]
	if l.Width() > 32 {
		t.Errorf("box largo %d colonne, atteso al più 32 (40%% di 80)", l.Width())
	}
	if l.Height() <= 3 {
		t.Errorf("testo lungo non spezzato: altezza %d", l.Height())
	}
	for _, line := range strings.Split(l.GetContent(), "\n") {
		if w := lipgloss.Width(line); w > 32 {
			t.Errorf("riga di %d colonne oltre il limite: %q", w, line)
		}
	}
}

// TestMessaggiNonIntercettati: Update ignora i messaggi non suoi (un tasto non
// cambia la pila; solo DismissErrors la tocca).
func TestMessaggiNonIntercettati(t *testing.T) {
	m := newTest(t)
	m.Push(Error, "x")
	m.Update(tea.KeyPressMsg{Code: 'a'})
	if got := m.Texts(); len(got) != 1 {
		t.Fatalf("un KeyPressMsg ha cambiato la pila: %q", got)
	}
}
