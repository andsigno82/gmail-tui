package thread

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/msg/richtext"
)

// Piano 15, Task 14 (D15-T5): le email grandi si rendono fuori dal loop; finche il
// risultato non arriva si vede il testo semplice con un indicatore; i risultati
// stantii si scartano.

// bigModel: messaggio con HTML oltre la soglia di rendering sincrono.
func bigModel(t *testing.T, html string) Model {
	t.Helper()
	full := testMsg()
	full.BodyHTML = html
	full.BodyText = "testo semplice di riserva"
	m := newModel(t, full)
	m.SetSize(80, 40)
	return m
}

func bigHTML() string { return "<p>" + strings.Repeat("parola ", 6000) + "</p>" }

func rendered(t *testing.T, cmd tea.Cmd) RenderedMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("nessuna Cmd di rendering")
	}
	r, ok := cmd().(RenderedMsg)
	if !ok {
		t.Fatalf("la Cmd non produce RenderedMsg")
	}
	return r
}

func TestEmailGrandeFallbackEPoiCorpo(t *testing.T) {
	m := bigModel(t, bigHTML())
	v := stripANSI(m.View())
	if !strings.Contains(v, "rendering in corso") || !strings.Contains(v, "testo semplice di riserva") {
		t.Fatalf("senza indicatore e testo di riserva:\n%s", v)
	}
	r := rendered(t, m.RenderCmd())
	if r.ID != m.AnchorID() || r.Width != 80 || r.Gen != m.gen {
		t.Errorf("RenderedMsg = %+v", RenderedMsg{ID: r.ID, Width: r.Width, Gen: r.Gen})
	}
	if m.RenderCmd() != nil {
		t.Errorf("la coda non e stata svuotata")
	}
	m, _ = m.Update(r)
	v = stripANSI(m.View())
	if strings.Contains(v, "rendering in corso") || !strings.Contains(v, "parola parola") {
		t.Errorf("dopo RenderedMsg il corpo a blocchi non e visibile:\n%s", v)
	}
}

func TestRenderedMsgStantioScartato(t *testing.T) {
	m := bigModel(t, bigHTML())
	r := rendered(t, m.RenderCmd())
	r.Gen++ // generazione diversa: thread cambiato nel frattempo
	m, _ = m.Update(r)
	if !strings.Contains(stripANSI(m.View()), "rendering in corso") {
		t.Errorf("un RenderedMsg con Gen vecchio ha cambiato la vista")
	}
}

// Cambio di larghezza = nuova generazione: il risultato vecchio e scartato e ne
// parte uno nuovo, uno solo.
func TestCambioLarghezzaScartaERilancia(t *testing.T) {
	m := bigModel(t, bigHTML())
	old := rendered(t, m.RenderCmd())
	m.SetSize(60, 40)
	m, _ = m.Update(old)
	if !strings.Contains(stripANSI(m.View()), "rendering in corso") {
		t.Fatalf("risultato per la larghezza precedente applicato")
	}
	new := rendered(t, m.RenderCmd())
	if new.Width != 60 || new.Gen == old.Gen {
		t.Errorf("nuovo rendering: larghezza %d, gen %d (vecchia %d)", new.Width, new.Gen, old.Gen)
	}
	m.SetSize(60, 40) // stessa larghezza: non rilancia il lavoro in volo
	if m.RenderCmd() != nil {
		t.Errorf("rendering duplicato per la stessa chiave in volo")
	}
}

// Il thread chiuso (Model zero) ignora i risultati in arrivo.
func TestRenderedMsgSuModelZeroIgnorato(t *testing.T) {
	m := bigModel(t, bigHTML())
	r := rendered(t, m.RenderCmd())
	var z Model
	z, cmd := z.Update(r)
	if cmd != nil || z.Loaded() {
		t.Errorf("il Model zero ha reagito a un RenderedMsg")
	}
}

// Email molto grande (oltre MaxInputBytes): non si rende nel loop e a render
// finito il corpo dichiara il troncamento.
func TestHTMLOltreLimiteDichiaraTroncamento(t *testing.T) {
	huge := "<p>" + strings.Repeat("parola ", richtext.MaxInputBytes/7+5000) + "</p>"
	m := bigModel(t, huge)
	if strings.Contains(stripANSI(m.View()), "[messaggio troncato") {
		t.Fatalf("il corpo grande e stato reso nel loop Update")
	}
	m, _ = m.Update(rendered(t, m.RenderCmd()))
	m.vp.GotoBottom()
	if v := stripANSI(m.View()); !strings.Contains(v, "[messaggio troncato: ") {
		t.Errorf("troncamento non dichiarato:\n%s", v)
	}
}

// Corpo di una conversazione grande: ogni messaggio ha il suo rendering.
func TestConversazioneConCorpoGrande(t *testing.T) {
	full := testMsg()
	full.BodyHTML = bigHTML()
	full.BodyText = "riserva"
	other := testMsg()
	other.Ref.ID = msg.NewID("INBOX", 8)
	m := NewConversation(full.Ref, []*msg.Message{other, full}, testStyles(t))
	m.SetSize(80, 40)
	cmd := m.RenderCmd()
	if cmd == nil {
		t.Fatal("nessun rendering accodato per il messaggio grande")
	}
	m, _ = m.Update(rendered(t, cmd))
	if strings.Contains(stripANSI(m.View()), "rendering in corso") {
		t.Errorf("rendering non applicato nella conversazione")
	}
}
