package thread

import (
	"fmt"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/render"
)

// Piano 15, Task 14 (D15-T2): cache di rendering a 16 voci, chiave
// (MessageID, larghezza, tema, variante).

func outWith(s string) render.Out {
	return render.Out{Lines: []string{s}, Zones: [][]render.Zone{nil}}
}

func TestRenderCacheStessaChiaveNonRicalcola(t *testing.T) {
	c := newRenderCache()
	n := 0
	compute := func() render.Out { n++; return outWith("x") }
	k := cacheKey{id: "a", width: 80, theme: "t", variant: "html"}

	c.lookup(k, compute)
	c.lookup(k, compute)
	if n != 1 {
		t.Fatalf("calcoli = %d, voglio 1 (stessa chiave)", n)
	}
	for name, other := range map[string]cacheKey{
		"larghezza": {id: "a", width: 60, theme: "t", variant: "html"},
		"tema":      {id: "a", width: 80, theme: "u", variant: "html"},
		"variante":  {id: "a", width: 80, theme: "t", variant: "text"},
		"messaggio": {id: "b", width: 80, theme: "t", variant: "html"},
	} {
		before := n
		c.lookup(other, compute)
		if n != before+1 {
			t.Errorf("chiave con %s diversa non ricalcola", name)
		}
	}
}

func TestRenderCacheLa17Espelle(t *testing.T) {
	c := newRenderCache()
	key := func(i int) cacheKey { return cacheKey{id: msg.MessageID(fmt.Sprint(i)), width: 80} }
	for i := range renderCacheMax + 1 {
		c.put(key(i), outWith(fmt.Sprint(i)))
	}
	if len(c.m) != renderCacheMax {
		t.Fatalf("voci = %d, voglio %d", len(c.m), renderCacheMax)
	}
	if _, ok := c.get(key(0)); ok {
		t.Errorf("la voce piu vecchia non e stata espulsa")
	}
	if _, ok := c.get(key(1)); !ok {
		t.Errorf("la seconda voce non doveva essere espulsa")
	}
	if _, ok := c.get(key(renderCacheMax)); !ok {
		t.Errorf("l'ultima voce manca")
	}
}

func TestRenderCacheRiscritturaNonCambiaOrdine(t *testing.T) {
	c := newRenderCache()
	k := func(i int) cacheKey { return cacheKey{id: msg.MessageID(fmt.Sprint(i))} }
	for i := range renderCacheMax {
		c.put(k(i), outWith("v1"))
	}
	c.put(k(0), outWith("v2")) // riscrive, resta la piu vecchia
	c.put(k(99), outWith("nuova"))
	if _, ok := c.get(k(0)); ok {
		t.Errorf("la voce riscritta resta la piu vecchia e va espulsa per prima")
	}
}

func TestRenderCacheNilSicura(t *testing.T) {
	var c *renderCache // Model zero: nessuna cache
	if _, ok := c.get(cacheKey{}); ok {
		t.Error("get su cache nil trova una voce")
	}
	c.put(cacheKey{}, outWith("x"))
	c.donePending(cacheKey{})
	c.resetPending()
	if c.markPending(cacheKey{}) {
		t.Error("markPending su cache nil deve essere false")
	}
}

// Il Model riusa il corpo gia reso: due richieste della stessa chiave danno la
// stessa memoria.
func TestModelRiusaIlCorpoInCache(t *testing.T) {
	full := testMsg()
	m := newModel(t, full)
	m.SetSize(80, 30)
	a := m.bodyOut(full)
	b := m.bodyOut(full)
	if len(a.Lines) == 0 || &a.Lines[0] != &b.Lines[0] {
		t.Errorf("corpo ricalcolato invece che dalla cache")
	}
	m.SetSize(60, 30)
	c := m.bodyOut(full)
	if &a.Lines[0] == &c.Lines[0] {
		t.Errorf("larghezza diversa: serve un nuovo rendering")
	}
}
