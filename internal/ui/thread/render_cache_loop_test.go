package thread

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Revisione Piano 15, punto 6: la cache deve valere tra messaggi e non espellere
// le voci dei corpi espansi in uso (altrimenti, con piu di renderCacheMax corpi
// grandi, il rendering in background si ripete all'infinito).

// runCmd esegue una Cmd (anche un Batch) e ne raccoglie i messaggi.
func runCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch v := cmd().(type) {
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range v {
			out = append(out, runCmd(c)...)
		}
		return out
	case nil:
		return nil
	default:
		return []tea.Msg{v}
	}
}

// driveRender fa girare il rendering in background come la radice: svuota la
// coda, applica i RenderedMsg, ripete finche non resta nulla da fare. Ritorna i
// giri fatti; piu di maxRounds = ciclo infinito.
func driveRender(m *Model, maxRounds int) (rounds int, loops bool) {
	for {
		msgs := runCmd(m.RenderCmd())
		if len(msgs) == 0 {
			return rounds, false
		}
		if rounds++; rounds > maxRounds {
			return rounds, true
		}
		for _, x := range msgs {
			if r, ok := x.(RenderedMsg); ok {
				m.applyRendered(r)
			}
		}
	}
}

// bigConversation: n messaggi non letti (tutti espansi) con un corpo HTML grande
// e diverso ciascuno.
func bigConversation(t *testing.T, n int, rc *RenderCache) Model {
	t.Helper()
	refs := make([]msg.MessageRef, n)
	bodies := make(map[msg.MessageID]*msg.Message, n)
	for i := range n {
		id := msg.MessageID(fmt.Sprintf("big-%d", i))
		ref := testRef()
		ref.ID, ref.Unread = id, true
		refs[i] = ref
		bodies[id] = &msg.Message{Ref: ref, BodyText: "riserva",
			BodyHTML: fmt.Sprintf("<p>messaggio %d %s</p>", i, strings.Repeat("parola ", 6000))}
	}
	m := NewLazyCached(refs[n-1], refs, bodies, testStyles(t), rc)
	m.SetSize(80, 40)
	return m
}

// Piu di 16 corpi grandi espansi: il rendering converge (nessuna espulsione di un
// corpo in uso, quindi nessun ri-rendering) e alla fine nessuno e in attesa.
func TestRenderCacheNonEspelleCorpiInUso(t *testing.T) {
	m := bigConversation(t, renderCacheMax+4, nil)
	rounds, loops := driveRender(&m, 10)
	if loops {
		t.Fatalf("il rendering in background non converge (%d giri): ciclo infinito per espulsione dei corpi in uso", rounds)
	}
	if v := stripANSI(m.View()); strings.Contains(v, "rendering in corso") {
		t.Errorf("un corpo e ancora in attesa dopo %d giri", rounds)
	}
	if rounds > 2 {
		t.Errorf("giri = %d, voglio al piu 2 (un solo rendering per corpo)", rounds)
	}
}

// La cache condivisa vale tra thread diversi: riaprire gli stessi messaggi (o
// passare a un altro thread e tornare) non rilancia il rendering.
func TestRenderCacheCondivisaTraThread(t *testing.T) {
	rc := NewRenderCache()
	a := bigConversation(t, 3, rc)
	if _, loops := driveRender(&a, 10); loops {
		t.Fatal("ciclo")
	}
	b := bigConversation(t, 3, rc) // stessi messaggi, nuovo Model
	if cmd := b.RenderCmd(); cmd != nil {
		t.Fatalf("secondo thread: rendering rilanciato nonostante la cache condivisa")
	}
	if v := stripANSI(b.View()); strings.Contains(v, "rendering in corso") {
		t.Errorf("secondo thread mostra il fallback:\n%s", v)
	}
}
